package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestCrawlStartAndGet(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Start a crawl (without OpenAPI — will complete immediately as stub)
	body := `{"entry_url":"https://example.com","depth":2,"same_origin":true,"blocklist":["/logout"],"limit":100}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	crawlID := created["id"].(string)
	if crawlID == "" {
		t.Fatal("expected non-empty crawl ID")
	}

	// Get crawl
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/crawls/"+crawlID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}

	var fetched map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&fetched)
	resp.Body.Close()

	if fetched["entry_url"] != "https://example.com" {
		t.Fatalf("expected entry_url 'https://example.com', got %v", fetched["entry_url"])
	}
}

func TestCrawlGetGraph(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"entry_url":"https://example.com"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	crawlID := created["id"].(string)

	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/crawls/"+crawlID+"/graph", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("graph: expected 200, got %d", resp.StatusCode)
	}

	var graph map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&graph)
	resp.Body.Close()

	if graph["nodes"] == nil {
		t.Fatal("expected nodes in graph")
	}
}

func TestCrawlGeneratePlan(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"entry_url":"https://example.com"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	crawlID := created["id"].(string)

	// Generate plan
	req, _ = http.NewRequest("POST", ts.URL()+"/api/v1/crawls/"+crawlID+"/generate-plan", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("generate-plan: expected 201, got %d", resp.StatusCode)
	}

	var plan map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&plan)
	resp.Body.Close()

	if plan["id"] == nil || plan["id"] == "" {
		t.Fatal("expected plan ID")
	}
	if plan["name"] == nil {
		t.Fatal("expected plan name")
	}
}

func TestCrawlNotFound(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/crawls/nonexistent", nil)
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCrawlCancel(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"entry_url":"https://example.com"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	crawlID := created["id"].(string)

	req, _ = http.NewRequest("POST", ts.URL()+"/api/v1/crawls/"+crawlID+"/cancel", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCrawlMissingURL(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
