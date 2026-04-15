package crawler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// ZAPEngine implements CrawlEngine using an OWASP ZAP sidecar's REST API.
type ZAPEngine struct {
	// BaseURL can be overridden for testing. Defaults to ZAP_API_URL env or http://zap:8080.
	BaseURL string
}

func (e *ZAPEngine) Name() string { return "zap" }

func (e *ZAPEngine) zapURL() string {
	if e.BaseURL != "" {
		return strings.TrimRight(e.BaseURL, "/")
	}
	if env := os.Getenv("ZAP_API_URL"); env != "" {
		return strings.TrimRight(env, "/")
	}
	return "http://zap:8080"
}

func (e *ZAPEngine) Crawl(ctx context.Context, cfg CrawlConfig, onProgress ProgressCallback) (db.CrawlGraph, []db.CapturedRequest, error) {
	base := e.zapURL()
	client := &http.Client{Timeout: 10 * time.Second}

	// Verify ZAP is reachable
	if err := e.ping(client, base); err != nil {
		return db.CrawlGraph{}, nil, fmt.Errorf("ZAP sidecar unreachable at %s: %w (start with: docker compose --profile security up)", base, err)
	}

	// Set up auth context in ZAP if needed
	if cfg.Auth.Type != "" && cfg.Auth.Type != "none" {
		if err := e.setupAuth(client, base, cfg); err != nil {
			return db.CrawlGraph{}, nil, fmt.Errorf("ZAP auth setup: %w", err)
		}
	}

	// Start spider scan
	scanID, err := e.startSpider(client, base, cfg.EntryURL, cfg.Depth)
	if err != nil {
		return db.CrawlGraph{}, nil, fmt.Errorf("ZAP spider start: %w", err)
	}

	// Poll spider status
	for {
		select {
		case <-ctx.Done():
			_ = e.stopSpider(client, base, scanID)
			return db.CrawlGraph{}, nil, ctx.Err()
		default:
		}

		status, err := e.spiderStatus(client, base, scanID)
		if err != nil {
			return db.CrawlGraph{}, nil, fmt.Errorf("ZAP spider status: %w", err)
		}

		if onProgress != nil {
			msgs, _ := e.getMessages(client, base, 0, 100)
			graph, reqs := messagesToGraphAndRequests(msgs, cfg)
			onProgress(
				db.CrawlProgress{PagesDiscovered: len(graph.Nodes), RequestsCaptured: len(reqs)},
				graph,
				reqs,
			)
		}

		if status >= 100 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	// Fetch all messages
	msgs, err := e.getMessages(client, base, 0, cfg.Limit)
	if err != nil {
		return db.CrawlGraph{}, nil, fmt.Errorf("ZAP get messages: %w", err)
	}

	graph, reqs := messagesToGraphAndRequests(msgs, cfg)
	return graph, reqs, nil
}

func (e *ZAPEngine) ping(client *http.Client, base string) error {
	resp, err := client.Get(base + "/JSON/core/view/version/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("ZAP returned status %d", resp.StatusCode)
	}
	return nil
}

func (e *ZAPEngine) startSpider(client *http.Client, base, targetURL string, maxDepth int) (string, error) {
	params := url.Values{
		"url":          {targetURL},
		"maxChildren":  {"0"},
		"recurse":      {"true"},
		"subtreeOnly":  {"true"},
	}
	if maxDepth > 0 {
		params.Set("maxDepth", strconv.Itoa(maxDepth))
	}

	resp, err := client.Get(base + "/JSON/spider/action/scan/?" + params.Encode())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Scan string `json:"scan"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Scan, nil
}

func (e *ZAPEngine) spiderStatus(client *http.Client, base, scanID string) (int, error) {
	resp, err := client.Get(base + "/JSON/spider/view/status/?scanId=" + scanID)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	status, _ := strconv.Atoi(result.Status)
	return status, nil
}

func (e *ZAPEngine) stopSpider(client *http.Client, base, scanID string) error {
	resp, err := client.Get(base + "/JSON/spider/action/stop/?scanId=" + scanID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type zapMessage struct {
	ID             string `json:"id"`
	Method         string `json:"method"`
	URL            string `json:"url"`
	RequestHeader  string `json:"requestHeader"`
	RequestBody    string `json:"requestBody"`
	ResponseHeader string `json:"responseHeader"`
	StatusCode     int    `json:"statusCode"`
}

func (e *ZAPEngine) getMessages(client *http.Client, base string, start, count int) ([]zapMessage, error) {
	params := url.Values{
		"start": {strconv.Itoa(start)},
		"count": {strconv.Itoa(count)},
	}

	resp, err := client.Get(base + "/JSON/core/view/messages/?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, err
	}

	var result struct {
		Messages []zapMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func (e *ZAPEngine) setupAuth(client *http.Client, base string, cfg CrawlConfig) error {
	switch cfg.Auth.Type {
	case "bearer":
		// Add a global replacer for Authorization header
		params := url.Values{
			"description":    {"BearerAuth"},
			"enabled":        {"true"},
			"matchType":      {"REQ_HEADER"},
			"matchRegex":     {"false"},
			"matchString":    {"Authorization:"},
			"replacement":    {"Authorization: Bearer " + cfg.Auth.Token},
			"initiators":     {""},
			"url":            {""},
		}
		resp, err := client.Get(base + "/JSON/replacer/action/addRule/?" + params.Encode())
		if err != nil {
			return err
		}
		resp.Body.Close()
	case "basic":
		encoded := "Basic " + basicB64(cfg.Auth.Username, cfg.Auth.Password)
		params := url.Values{
			"description":    {"BasicAuth"},
			"enabled":        {"true"},
			"matchType":      {"REQ_HEADER"},
			"matchRegex":     {"false"},
			"matchString":    {"Authorization:"},
			"replacement":    {"Authorization: " + encoded},
			"initiators":     {""},
			"url":            {""},
		}
		resp, err := client.Get(base + "/JSON/replacer/action/addRule/?" + params.Encode())
		if err != nil {
			return err
		}
		resp.Body.Close()
	case "form_login":
		// ZAP form-based auth requires creating an auth context — simplified approach
		params := url.Values{
			"url":          {cfg.EntryURL},
			"requestData":  {"username=" + url.QueryEscape(cfg.Auth.Username) + "&password=" + url.QueryEscape(cfg.Auth.Password)},
		}
		resp, err := client.Get(base + "/JSON/authentication/action/setAuthenticationMethod/?" + params.Encode())
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func basicB64(user, pass string) string {
	return base64Encode(user + ":" + pass)
}

func base64Encode(s string) string {
	data := []byte(s)
	return base64.StdEncoding.EncodeToString(data)
}

func messagesToGraphAndRequests(msgs []zapMessage, cfg CrawlConfig) (db.CrawlGraph, []db.CapturedRequest) {
	nodeSet := map[string]bool{}
	var nodes []db.CrawlGraphNode
	var edges []db.CrawlGraphEdge
	var requests []db.CapturedRequest

	for _, msg := range msgs {
		requests = append(requests, db.CapturedRequest{
			Method:     strings.ToUpper(msg.Method),
			URL:        msg.URL,
			Body:       msg.RequestBody,
			StatusCode: msg.StatusCode,
		})

		if !nodeSet[msg.URL] {
			nodeSet[msg.URL] = true
			nodes = append(nodes, db.CrawlGraphNode{URL: msg.URL, Title: msg.URL})
		}
	}

	// Build edges from sequential request pairs (simplified site graph)
	for i := 1; i < len(msgs); i++ {
		if msgs[i-1].URL != msgs[i].URL {
			edges = append(edges, db.CrawlGraphEdge{
				Source: msgs[i-1].URL,
				Target: msgs[i].URL,
			})
		}
	}

	return db.CrawlGraph{
		Nodes: ensureNodes(nodes),
		Edges: ensureEdges(edges),
	}, ensureRequests(requests)
}
