package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// A crawl's password and token are used while it runs but never returned.
func TestCrawlDoesNotReturnCredentials(t *testing.T) {
	ts := testutil.NewTestServer(t)

	for _, auth := range []string{
		`{"type":"form_login","username":"demo","password":"s3cret-pass"}`,
		`{"type":"bearer","token":"s3cret-token"}`,
	} {
		body := `{"entry_url":"http://127.0.0.1:1/","engine":"colly","depth":1,"auth":` + auth + `}`
		req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp := ts.Do(req)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("start: expected 201, got %d: %s", resp.StatusCode, raw)
		}
		var created map[string]interface{}
		json.Unmarshal(raw, &created)

		req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/crawls/"+created["id"].(string), nil)
		resp = ts.Do(req)
		fetched, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		for _, out := range [][]byte{raw, fetched} {
			if bytes.Contains(out, []byte("s3cret")) {
				t.Fatalf("crawl response contains a credential: %s", out)
			}
		}
		if !bytes.Contains(fetched, []byte(`"type":`)) {
			t.Fatalf("expected the auth type to be kept: %s", fetched)
		}
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

// An OpenAPI import of a YAML document generates requests against the API's
// server (the one on the document's host), with examples filled in.
func TestCrawlOpenAPIImportYAMLGeneratesRunnablePlan(t *testing.T) {
	var api *httptest.Server
	api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		fmt.Fprintf(w, `openapi: 3.0.3
info:
  title: Shop
  version: "1"
servers:
  - url: http://localhost:9002
  - url: %s
paths:
  /products/{id}:
    get:
      summary: Get product
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            example: p1
      responses:
        200:
          description: ok
  /orders:
    post:
      summary: Create order
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/NewOrder'
      responses:
        201:
          description: created
components:
  schemas:
    NewOrder:
      type: object
      properties:
        product_ids:
          type: array
          items:
            type: string
          example: ["p1", "p2"]
`, api.URL)
	}))
	defer api.Close()

	ts := testutil.NewTestServer(t)
	body := fmt.Sprintf(`{"openapi_url":%q}`, api.URL+"/openapi.yaml")
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/crawls", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var created struct{ ID string }
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	var crawl struct {
		Status       string
		ErrorMessage string `json:"error_message"`
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/crawls/"+created.ID, nil)
		resp = ts.Do(req)
		json.NewDecoder(resp.Body).Decode(&crawl)
		resp.Body.Close()
		if crawl.Status != "pending" && crawl.Status != "crawling" {
			break
		}
	}
	if crawl.Status != "completed" {
		t.Fatalf("import status %q (%s), want completed", crawl.Status, crawl.ErrorMessage)
	}

	req, _ = http.NewRequest("POST", ts.URL()+"/api/v1/crawls/"+created.ID+"/generate-plan", nil)
	resp = ts.Do(req)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("generate-plan: got %d", resp.StatusCode)
	}
	var plan struct {
		Root struct {
			Children []struct {
				Children []struct {
					Properties struct {
						Method string
						URL    string
						Body   struct{ Type, Content string }
					}
				}
			}
		}
	}
	json.NewDecoder(resp.Body).Decode(&plan)
	resp.Body.Close()

	got := map[string]string{}
	for _, n := range plan.Root.Children[0].Children {
		got[n.Properties.Method+" "+n.Properties.URL] = n.Properties.Body.Content
	}
	if _, ok := got["GET "+api.URL+"/products/p1"]; !ok {
		t.Errorf("GET request should use the document host and the path example; got %v", got)
	}
	orderBody, ok := got["POST "+api.URL+"/orders"]
	if !ok || !strings.Contains(orderBody, `"p2"`) {
		t.Errorf("POST /orders should carry an example body from the $ref schema; got %v", got)
	}
}
