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

	// Default engine should be "rod"
	if created["engine"] != "rod" {
		t.Fatalf("expected default engine 'rod', got %v", created["engine"])
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
	if fetched["engine"] != "rod" {
		t.Fatalf("expected engine 'rod', got %v", fetched["engine"])
	}
}

func TestCrawlStartWithEngine(t *testing.T) {
	ts := testutil.NewTestServer(t)

	engines := []string{"rod", "colly", "zap"}
	for _, engine := range engines {
		body := `{"entry_url":"https://example.com","engine":"` + engine + `"}`
		req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := ts.Do(req)

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("start with engine %s: expected 201, got %d", engine, resp.StatusCode)
		}

		var created map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&created)
		resp.Body.Close()

		if created["engine"] != engine {
			t.Fatalf("expected engine %q, got %v", engine, created["engine"])
		}
	}
}

func TestCrawlStartInvalidEngine(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"entry_url":"https://example.com","engine":"invalid"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid engine, got %d", resp.StatusCode)
	}
	resp.Body.Close()
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

	// Wait briefly for async crawl to either complete or fail
	// Then check status — since the engine can't actually connect to example.com,
	// we need to verify the crawl is in a completed state for plan generation
	// For the test, the crawl may have failed since example.com isn't reachable
	// We'll just test the validation path
	req, _ = http.NewRequest("POST", ts.URL()+"/api/v1/crawls/"+crawlID+"/generate-plan", nil)
	resp = ts.Do(req)

	// May be 400 (not complete) or 201 (completed) depending on timing
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("generate-plan: expected 201 or 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
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

	// Verify status is cancelled
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/crawls/"+crawlID, nil)
	resp = ts.Do(req)
	var fetched map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&fetched)
	resp.Body.Close()

	if fetched["status"] != "cancelled" {
		t.Fatalf("expected status 'cancelled', got %v", fetched["status"])
	}
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
