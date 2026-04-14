package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestEnvironmentCRUD(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := `{"name":"Staging","variables":{"BASE_URL":{"value":"https://staging.example.com","secret":false},"API_KEY":{"value":"secret123","secret":true}}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/environments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	envID := created["id"].(string)
	if envID == "" {
		t.Fatal("create: expected non-empty id")
	}
	if created["name"] != "Staging" {
		t.Fatalf("create: expected name 'Staging', got %v", created["name"])
	}

	// List
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/environments", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var listResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResult)
	resp.Body.Close()

	items := listResult["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("list: expected 1 item, got %d", len(items))
	}

	// Get (should have redacted secrets)
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/environments/"+envID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}

	var fetched map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&fetched)
	resp.Body.Close()

	vars := fetched["variables"].(map[string]interface{})
	apiKey := vars["API_KEY"].(map[string]interface{})
	if apiKey["value"] != "***REDACTED***" {
		t.Fatalf("get: expected redacted API_KEY, got %v", apiKey["value"])
	}

	baseURL := vars["BASE_URL"].(map[string]interface{})
	if baseURL["value"] != "https://staging.example.com" {
		t.Fatalf("get: expected BASE_URL value, got %v", baseURL["value"])
	}

	// Update
	updateBody := `{"name":"Production","variables":{"BASE_URL":{"value":"https://prod.example.com","secret":false}}}`
	req, _ = http.NewRequest("PUT", ts.URL()+"/api/v1/environments/"+envID, bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}

	var updated map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()

	if updated["name"] != "Production" {
		t.Fatalf("update: expected name 'Production', got %v", updated["name"])
	}

	// Delete
	req, _ = http.NewRequest("DELETE", ts.URL()+"/api/v1/environments/"+envID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/environments/"+envID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEnvironmentCreateValidation(t *testing.T) {
	ts := testutil.NewTestServer(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "empty name",
			body:       `{"name":"","variables":{}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "invalid json",
			body:       `{bad json`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/environments", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp := ts.Do(req)

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}

			var errResp struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			json.NewDecoder(resp.Body).Decode(&errResp)
			resp.Body.Close()

			if errResp.Error.Code != tt.wantCode {
				t.Fatalf("expected error code %q, got %q", tt.wantCode, errResp.Error.Code)
			}
		})
	}
}

func TestEnvironmentNotFound(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/environments/nonexistent-id", nil)
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()

	if errResp.Error.Code != "ENVIRONMENT_NOT_FOUND" {
		t.Fatalf("expected ENVIRONMENT_NOT_FOUND, got %q", errResp.Error.Code)
	}
}
