package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestPlaygroundExecuteGet(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"method":"GET","url":"https://httpbin.org/get","headers":{},"body":"","body_type":"none","auth_type":"","auth_config":null}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/playground/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("execute: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	status := int(result["status"].(float64))
	if status != 200 {
		t.Fatalf("expected response status 200, got %d", status)
	}

	timing := result["timing"].(map[string]interface{})
	totalMS := timing["total_ms"].(float64)
	if totalMS <= 0 {
		t.Fatal("expected positive total_ms")
	}

	if result["body"] == nil || result["body"] == "" {
		t.Fatal("expected non-empty response body")
	}
}

func TestPlaygroundExecuteWithBearerAuth(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"method":"GET","url":"https://httpbin.org/headers","headers":{},"body":"","body_type":"none","auth_type":"bearer","auth_config":{"token":"test-token-123"}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/playground/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("execute: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	// httpbin echoes back headers — check our auth header was sent
	respBody := result["body"].(string)
	if !bytes.Contains([]byte(respBody), []byte("Bearer test-token-123")) {
		t.Fatal("expected Bearer token in echoed headers")
	}
}

func TestPlaygroundMissingURL(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"method":"GET","url":"","headers":{}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/playground/request", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPlaygroundCollectionsCRUD(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create collection
	body := `{"name":"My API","requests":[{"name":"Get Users","method":"GET","url":"https://api.example.com/users","headers":{},"body":"","body_type":"none","auth_type":"","auth_config":null}]}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/playground/collections", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	if created["name"] != "My API" {
		t.Fatalf("expected name 'My API', got %v", created["name"])
	}

	requests := created["requests"].([]interface{})
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}

	// List collections
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/playground/collections", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var listResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResult)
	resp.Body.Close()

	items := listResult["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(items))
	}
}
