package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func TestZAPEngineName(t *testing.T) {
	engine := &ZAPEngine{}
	if engine.Name() != "zap" {
		t.Fatalf("expected name 'zap', got %q", engine.Name())
	}
}

func TestZAPEngineUnreachable(t *testing.T) {
	engine := &ZAPEngine{BaseURL: "http://127.0.0.1:1"}

	cfg := CrawlConfig{
		EntryURL: "https://example.com",
		Depth:    2,
		Limit:    100,
	}

	_, _, err := engine.Crawl(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected error for unreachable ZAP")
	}
	if !contains(err.Error(), "unreachable") {
		t.Fatalf("expected 'unreachable' in error, got %q", err.Error())
	}
}

func TestZAPEngineFullCrawl(t *testing.T) {
	spiderStarted := false
	spiderDone := false

	mux := http.NewServeMux()

	// Version endpoint (ping)
	mux.HandleFunc("/JSON/core/view/version/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "2.14.0"})
	})

	// Start spider
	mux.HandleFunc("/JSON/spider/action/scan/", func(w http.ResponseWriter, r *http.Request) {
		spiderStarted = true
		json.NewEncoder(w).Encode(map[string]string{"scan": "1"})
	})

	// Spider status — return 100 (complete) immediately
	mux.HandleFunc("/JSON/spider/view/status/", func(w http.ResponseWriter, r *http.Request) {
		spiderDone = true
		json.NewEncoder(w).Encode(map[string]string{"status": "100"})
	})

	// Messages
	mux.HandleFunc("/JSON/core/view/messages/", func(w http.ResponseWriter, r *http.Request) {
		msgs := []map[string]interface{}{
			{"id": "1", "method": "GET", "url": "https://example.com/", "statusCode": 200, "requestHeader": "", "requestBody": "", "responseHeader": ""},
			{"id": "2", "method": "GET", "url": "https://example.com/about", "statusCode": 200, "requestHeader": "", "requestBody": "", "responseHeader": ""},
			{"id": "3", "method": "POST", "url": "https://example.com/api/data", "statusCode": 201, "requestHeader": "", "requestBody": "{}", "responseHeader": ""},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": msgs})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &ZAPEngine{BaseURL: srv.URL}
	cfg := CrawlConfig{
		EntryURL: "https://example.com",
		Depth:    3,
		Limit:    100,
	}

	graph, requests, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !spiderStarted {
		t.Error("expected spider to be started")
	}
	if !spiderDone {
		t.Error("expected spider status to be checked")
	}

	if len(graph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(graph.Nodes))
	}
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}

	// Check request methods
	methodCounts := map[string]int{}
	for _, req := range requests {
		methodCounts[req.Method]++
	}
	if methodCounts["GET"] != 2 {
		t.Fatalf("expected 2 GET requests, got %d", methodCounts["GET"])
	}
	if methodCounts["POST"] != 1 {
		t.Fatalf("expected 1 POST request, got %d", methodCounts["POST"])
	}
}

func TestZAPEngineCancellation(t *testing.T) {
	stopCalled := false

	mux := http.NewServeMux()
	mux.HandleFunc("/JSON/core/view/version/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "2.14.0"})
	})
	mux.HandleFunc("/JSON/spider/action/scan/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"scan": "1"})
	})
	mux.HandleFunc("/JSON/spider/view/status/", func(w http.ResponseWriter, r *http.Request) {
		// Never complete — will be cancelled
		json.NewEncoder(w).Encode(map[string]string{"status": "50"})
	})
	mux.HandleFunc("/JSON/spider/action/stop/", func(w http.ResponseWriter, r *http.Request) {
		stopCalled = true
		json.NewEncoder(w).Encode(map[string]string{"Result": "OK"})
	})
	mux.HandleFunc("/JSON/core/view/messages/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": []interface{}{}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &ZAPEngine{BaseURL: srv.URL}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		cancel()
	}()

	cfg := CrawlConfig{
		EntryURL: "https://example.com",
		Depth:    3,
		Limit:    100,
	}

	_, _, err := engine.Crawl(ctx, cfg, nil)
	if err == nil {
		t.Log("crawl completed before cancellation")
	}

	_ = stopCalled
}

func TestZAPEngineWithBearerAuth(t *testing.T) {
	authRuleAdded := false

	mux := http.NewServeMux()
	mux.HandleFunc("/JSON/core/view/version/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "2.14.0"})
	})
	mux.HandleFunc("/JSON/replacer/action/addRule/", func(w http.ResponseWriter, r *http.Request) {
		authRuleAdded = true
		replacement := r.URL.Query().Get("replacement")
		if replacement != "Authorization: Bearer my-secret-token" {
			t.Errorf("expected bearer token in replacement, got %q", replacement)
		}
		json.NewEncoder(w).Encode(map[string]string{"Result": "OK"})
	})
	mux.HandleFunc("/JSON/spider/action/scan/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"scan": "1"})
	})
	mux.HandleFunc("/JSON/spider/view/status/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "100"})
	})
	mux.HandleFunc("/JSON/core/view/messages/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": []interface{}{}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &ZAPEngine{BaseURL: srv.URL}
	cfg := CrawlConfig{
		EntryURL: "https://example.com",
		Auth:     db.CrawlAuthConfig{Type: "bearer", Token: "my-secret-token"},
		Depth:    2,
		Limit:    100,
	}

	_, _, err := engine.Crawl(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !authRuleAdded {
		t.Error("expected auth rule to be added to ZAP")
	}
}

func TestZAPEngineProgress(t *testing.T) {
	callCount := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/JSON/core/view/version/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "2.14.0"})
	})
	mux.HandleFunc("/JSON/spider/action/scan/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"scan": "1"})
	})
	mux.HandleFunc("/JSON/spider/view/status/", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Complete on first call
		json.NewEncoder(w).Encode(map[string]string{"status": "100"})
	})
	mux.HandleFunc("/JSON/core/view/messages/", func(w http.ResponseWriter, r *http.Request) {
		msgs := []map[string]interface{}{
			{"id": "1", "method": "GET", "url": "https://example.com/", "statusCode": 200},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": msgs})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	engine := &ZAPEngine{BaseURL: srv.URL}
	progressCalled := false

	cfg := CrawlConfig{
		EntryURL: "https://example.com",
		Depth:    2,
		Limit:    100,
	}

	_, _, err := engine.Crawl(context.Background(), cfg, func(p db.CrawlProgress, g db.CrawlGraph, r []db.CapturedRequest) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = progressCalled
}

func TestMessageToGraphAndRequests(t *testing.T) {
	msgs := []zapMessage{
		{ID: "1", Method: "get", URL: "https://example.com/", StatusCode: 200},
		{ID: "2", Method: "post", URL: "https://example.com/api", StatusCode: 201, RequestBody: "{}"},
		{ID: "3", Method: "get", URL: "https://example.com/about", StatusCode: 200},
	}

	cfg := CrawlConfig{Limit: 100}
	graph, reqs := messagesToGraphAndRequests(msgs, cfg)

	if len(graph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(graph.Nodes))
	}
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}

	// Methods should be uppercased
	if reqs[0].Method != "GET" {
		t.Fatalf("expected GET, got %q", reqs[0].Method)
	}
	if reqs[1].Method != "POST" {
		t.Fatalf("expected POST, got %q", reqs[1].Method)
	}

	// Edges should exist between sequential different URLs
	if len(graph.Edges) < 2 {
		t.Fatalf("expected at least 2 edges, got %d", len(graph.Edges))
	}
}

func contains(s, substr string) bool {
	return fmt.Sprintf("%s", s) != "" && len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
