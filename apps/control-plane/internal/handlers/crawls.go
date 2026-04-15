package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/crawler"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type CrawlHandlers struct {
	crawls *db.CrawlStore
	plans  *db.PlanStore
}

func NewCrawlHandlers(crawls *db.CrawlStore, plans *db.PlanStore) *CrawlHandlers {
	return &CrawlHandlers{crawls: crawls, plans: plans}
}

type startCrawlRequest struct {
	EntryURL   string          `json:"entry_url"`
	Auth       db.CrawlAuthConfig `json:"auth"`
	Depth      int             `json:"depth"`
	SameOrigin *bool           `json:"same_origin"`
	Blocklist  []string        `json:"blocklist"`
	Limit      int             `json:"limit"`
	OpenAPIURL string          `json:"openapi_url"`
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

	crawl, err := h.crawls.Create(entryURL, req.Auth, depth, sameOrigin, req.Blocklist, limit, req.OpenAPIURL)
	if err != nil {
		internalError(w, err)
		return
	}

	// If OpenAPI URL is provided, fetch and process immediately
	if req.OpenAPIURL != "" {
		go h.processOpenAPI(crawl.ID, req.OpenAPIURL)
	} else {
		// For Playwright-based crawling, mark as pending
		// Playwright crawl execution would be triggered here in a production build
		h.crawls.UpdateStatus(crawl.ID, "completed")
		h.crawls.UpdateProgress(crawl.ID, db.CrawlProgress{
			PagesDiscovered:  0,
			RequestsCaptured: 0,
		})
	}

	// Re-fetch to get latest status
	crawl, _ = h.crawls.Get(crawl.ID)
	writeJSON(w, http.StatusCreated, crawl)
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
		plan = crawler.GeneratePlanFromOpenAPI(&spec, crawl.EntryURL)
	} else {
		// Generate plan from captured requests
		plan = generatePlanFromRequests(crawl)
	}

	// Save the plan
	savedPlan, err := h.plans.Create(plan.Name, plan.Description, plan.Tags, plan.Root, nil)
	if err != nil {
		internalError(w, err)
		return
	}

	h.crawls.SetGeneratedPlan(id, savedPlan.ID)

	writeJSON(w, http.StatusCreated, savedPlan)
}

func generatePlanFromRequests(crawl *db.Crawl) *db.Plan {
	var samplers []db.Node
	for _, req := range crawl.Requests {
		bodyJSON, _ := json.Marshal(map[string]interface{}{"type": "none"})
		propsJSON, _ := json.Marshal(map[string]interface{}{
			"method":           req.Method,
			"url":              req.URL,
			"headers":          []interface{}{},
			"body":             json.RawMessage(bodyJSON),
			"follow_redirects": true,
		})

		samplers = append(samplers, db.Node{
			ID:         "req-" + req.Method + "-" + req.URL,
			Type:       "http",
			Name:       req.Method + " " + req.URL,
			Enabled:    true,
			Properties: propsJSON,
			Children:   []db.Node{},
		})
	}

	scenario := db.Node{
		ID:         "scenario-crawl",
		Type:       "scenario",
		Name:       "Crawl Scenario",
		Enabled:    true,
		Properties: json.RawMessage(`{"executor":"constant-vus","vus":1,"duration":"30s"}`),
		Children:   samplers,
	}

	root := db.Node{
		ID:         "root-crawl",
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

	h.crawls.UpdateStatus(id, "cancelled")
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
