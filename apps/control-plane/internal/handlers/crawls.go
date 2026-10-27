package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/crawler"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func sortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type CrawlHandlers struct {
	crawls       *db.CrawlStore
	plans        *db.PlanStore
	activeCrawls sync.Map // map[string]context.CancelFunc
}

func NewCrawlHandlers(crawls *db.CrawlStore, plans *db.PlanStore) *CrawlHandlers {
	return &CrawlHandlers{crawls: crawls, plans: plans}
}

type startCrawlRequest struct {
	EntryURL   string             `json:"entry_url"`
	Engine     string             `json:"engine,omitempty"`
	Auth       db.CrawlAuthConfig `json:"auth,omitempty"`
	Depth      int                `json:"depth,omitempty"`
	SameOrigin *bool              `json:"same_origin"`
	Blocklist  []string           `json:"blocklist,omitempty"`
	Limit      int                `json:"limit,omitempty"`
	OpenAPIURL string             `json:"openapi_url,omitempty"`
}

func (h *CrawlHandlers) Start(w http.ResponseWriter, r *http.Request) {
	var req startCrawlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.EntryURL == "" && req.OpenAPIURL == "" {
		badRequest(w, "VALIDATION_ERROR", "entry_url or openapi_url is required", nil)
		return
	}

	// Validate engine
	engine := req.Engine
	if engine == "" {
		engine = "rod"
	}
	if engine != "rod" && engine != "colly" && engine != "zap" {
		badRequest(w, "INVALID_ENGINE", "engine must be one of: rod, colly, zap", nil)
		return
	}

	depth := req.Depth
	if depth <= 0 {
		depth = 3
	}
	sameOrigin := true
	if req.SameOrigin != nil {
		sameOrigin = *req.SameOrigin
	}
	if req.Blocklist == nil {
		req.Blocklist = []string{"/logout", "/delete"}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}

	entryURL := req.EntryURL
	if entryURL == "" {
		entryURL = req.OpenAPIURL
	}

	crawl, err := h.crawls.Create(entryURL, engine, req.Auth, depth, sameOrigin, req.Blocklist, limit, req.OpenAPIURL)
	if err != nil {
		internalError(w, err)
		return
	}

	// If OpenAPI URL is provided, fetch and process immediately
	if req.OpenAPIURL != "" {
		go h.processOpenAPI(crawl.ID, req.OpenAPIURL)
	} else {
		// Dispatch to the selected engine
		crawlEngine, engineErr := crawler.GetEngine(engine)
		if engineErr != nil {
			h.crawls.SetError(crawl.ID, engineErr.Error())
		} else {
			go h.runCrawl(crawl.ID, crawlEngine, crawler.CrawlConfig{
				EntryURL:   entryURL,
				Auth:       req.Auth,
				Depth:      depth,
				SameOrigin: sameOrigin,
				Blocklist:  req.Blocklist,
				Limit:      limit,
			})
		}
	}

	// Re-fetch to get latest status
	crawl, _ = h.crawls.Get(crawl.ID)
	writeJSON(w, http.StatusCreated, crawl)
}

// runCrawl executes a crawl engine in a background goroutine.
func (h *CrawlHandlers) runCrawl(crawlID string, engine crawler.CrawlEngine, cfg crawler.CrawlConfig) {
	ctx, cancel := context.WithCancel(context.Background())
	h.activeCrawls.Store(crawlID, cancel)
	defer func() {
		h.activeCrawls.Delete(crawlID)
		cancel()
	}()

	h.crawls.UpdateStatus(crawlID, "crawling")

	onProgress := func(progress db.CrawlProgress, graph db.CrawlGraph, requests []db.CapturedRequest) {
		h.crawls.UpdateProgress(crawlID, progress)
		h.crawls.UpdateGraph(crawlID, graph, requests)
	}

	graph, requests, err := engine.Crawl(ctx, cfg, onProgress)

	if ctx.Err() != nil {
		h.crawls.UpdateStatus(crawlID, "cancelled")
		return
	}

	if err != nil {
		h.crawls.SetError(crawlID, err.Error())
		return
	}

	h.crawls.UpdateGraph(crawlID, graph, requests)
	h.crawls.UpdateProgress(crawlID, db.CrawlProgress{
		PagesDiscovered:  len(graph.Nodes),
		RequestsCaptured: len(requests),
	})
	h.crawls.UpdateStatus(crawlID, "completed")
}

func (h *CrawlHandlers) processOpenAPI(crawlID, openapiURL string) {
	h.crawls.UpdateStatus(crawlID, "crawling")

	spec, err := crawler.FetchAndParse(openapiURL)
	if err != nil {
		h.crawls.SetError(crawlID, err.Error())
		return
	}

	specJSON, _ := json.Marshal(spec)
	h.crawls.UpdateOpenAPISpec(crawlID, string(specJSON))

	// Build graph from paths
	var nodes []db.CrawlGraphNode
	var edges []db.CrawlGraphEdge
	var requests []db.CapturedRequest

	for path, methods := range spec.Paths {
		nodes = append(nodes, db.CrawlGraphNode{URL: path, Title: path})
		for method, op := range methods {
			name := op.Summary
			if name == "" {
				name = op.OperationID
			}
			requests = append(requests, db.CapturedRequest{
				Method:     method,
				URL:        path,
				StatusCode: 200,
			})
			_ = name
		}
	}

	h.crawls.UpdateGraph(crawlID, db.CrawlGraph{Nodes: nodes, Edges: edges}, requests)
	h.crawls.UpdateProgress(crawlID, db.CrawlProgress{
		PagesDiscovered:  len(nodes),
		RequestsCaptured: len(requests),
	})
	h.crawls.UpdateStatus(crawlID, "completed")
}

func (h *CrawlHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	crawl, err := h.crawls.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if crawl == nil {
		notFound(w, "CRAWL", id)
		return
	}

	writeJSON(w, http.StatusOK, crawl)
}

func (h *CrawlHandlers) GetGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	crawl, err := h.crawls.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if crawl == nil {
		notFound(w, "CRAWL", id)
		return
	}

	writeJSON(w, http.StatusOK, crawl.Graph)
}

func (h *CrawlHandlers) GeneratePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	crawl, err := h.crawls.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if crawl == nil {
		notFound(w, "CRAWL", id)
		return
	}

	if crawl.Status != "completed" {
		badRequest(w, "CRAWL_NOT_COMPLETE", "Crawl must be completed before generating a plan", nil)
		return
	}

	// Generate plan from OpenAPI spec if available
	var plan *db.Plan
	if crawl.OpenAPISpec != "" {
		var spec crawler.OpenAPISpec
		json.Unmarshal([]byte(crawl.OpenAPISpec), &spec)
		specURL := crawl.OpenAPIURL
		if specURL == "" {
			specURL = crawl.EntryURL
		}
		// The entry URL of an import is the document itself, not the API's base.
		plan = crawler.GeneratePlanFromOpenAPI(&spec, crawler.ResolveBaseURL(&spec, specURL))
	} else {
		// Generate plan from captured requests
		plan = generatePlanFromRequests(crawl)
	}

	// Save the plan
	savedPlan, err := h.plans.Create(plan.Name, plan.Description, plan.Tags, plan.Root, nil, false)
	if err != nil {
		internalError(w, err)
		return
	}

	h.crawls.SetGeneratedPlan(id, savedPlan.ID)

	writeJSON(w, http.StatusCreated, savedPlan)
}

// generatePlanFromRequests turns captured requests into one HTTP node per
// unique method and URL, in the order they were first seen.
func generatePlanFromRequests(crawl *db.Crawl) *db.Plan {
	var samplers []db.Node
	seen := map[string]bool{}
	for _, req := range crawl.Requests {
		key := req.Method + " " + req.URL
		if seen[key] {
			continue
		}
		seen[key] = true

		body := map[string]interface{}{"type": "none"}
		var headers []map[string]interface{}
		if req.Body != "" {
			rawType := ""
			for k, v := range req.Headers {
				if strings.EqualFold(k, "Content-Type") {
					rawType = v
				}
			}
			contentType := strings.ToLower(rawType)
			switch {
			case strings.Contains(contentType, "json"):
				body = map[string]interface{}{"type": "json", "content": req.Body}
			case strings.Contains(contentType, "x-www-form-urlencoded") || contentType == "":
				if values, err := url.ParseQuery(req.Body); err == nil && len(values) > 0 {
					fields := []map[string]string{}
					for _, k := range sortedKeys(values) {
						for _, v := range values[k] {
							fields = append(fields, map[string]string{"key": k, "value": v})
						}
					}
					body = map[string]interface{}{"type": "form", "form_fields": fields}
				} else {
					body = map[string]interface{}{"type": "raw", "content": req.Body}
				}
			default:
				body = map[string]interface{}{"type": "raw", "content": req.Body}
				headers = append(headers, map[string]interface{}{"key": "Content-Type", "value": rawType, "enabled": true})
			}
		}
		if headers == nil {
			headers = []map[string]interface{}{}
		}
		propsJSON, _ := json.Marshal(map[string]interface{}{
			"method":           req.Method,
			"url":              req.URL,
			"headers":          headers,
			"body":             body,
			"follow_redirects": true,
		})

		name := req.URL
		if u, err := url.Parse(req.URL); err == nil && u.Path != "" {
			name = u.Path
			if u.RawQuery != "" {
				name += "?" + u.RawQuery
			}
		}
		samplers = append(samplers, db.Node{
			ID:         uuid.NewString(),
			Type:       "http",
			Name:       req.Method + " " + name,
			Enabled:    true,
			Properties: propsJSON,
			Children:   []db.Node{},
		})
	}

	scenario := db.Node{
		ID:         uuid.NewString(),
		Type:       "scenario",
		Name:       "Crawl Scenario",
		Enabled:    true,
		Properties: json.RawMessage(`{"executor":"constant-vus","vus":1,"duration":"30s"}`),
		Children:   samplers,
	}

	root := db.Node{
		ID:         uuid.NewString(),
		Type:       "plan",
		Name:       "Crawl: " + crawl.EntryURL,
		Enabled:    true,
		Properties: json.RawMessage(`{}`),
		Children:   []db.Node{scenario},
	}

	return &db.Plan{
		Name:        "Crawl: " + crawl.EntryURL,
		Description: "Auto-generated from crawl",
		Tags:        []string{"auto-generated", "crawl"},
		Root:        root,
	}
}

func (h *CrawlHandlers) Cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	crawl, err := h.crawls.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if crawl == nil {
		notFound(w, "CRAWL", id)
		return
	}

	// Cancel the active crawl context if running
	if cancelFn, ok := h.activeCrawls.LoadAndDelete(id); ok {
		cancelFn.(context.CancelFunc)()
	}

	h.crawls.UpdateStatus(id, "cancelled")
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
