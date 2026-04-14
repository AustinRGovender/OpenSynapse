package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestPlanCRUD(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := `{"name":"My Plan","description":"A test plan","tags":["smoke"],"root":{"id":"r1","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/plans", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	planID := created["id"].(string)
	if planID == "" {
		t.Fatal("create: expected non-empty id")
	}
	if created["name"] != "My Plan" {
		t.Fatalf("create: expected name 'My Plan', got %v", created["name"])
	}
	if created["version"].(float64) != 1 {
		t.Fatalf("create: expected version 1, got %v", created["version"])
	}

	// List
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/plans", nil)
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

	// Get
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/plans/"+planID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}

	var fetched map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&fetched)
	resp.Body.Close()

	if fetched["id"] != planID {
		t.Fatalf("get: expected id %s, got %v", planID, fetched["id"])
	}

	// Update
	updateBody := `{"name":"Updated Plan","description":"Updated","tags":["load"],"root":{"id":"r1","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ = http.NewRequest("PUT", ts.URL()+"/api/v1/plans/"+planID, bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}

	var updated map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&updated)
	resp.Body.Close()

	if updated["name"] != "Updated Plan" {
		t.Fatalf("update: expected name 'Updated Plan', got %v", updated["name"])
	}
	if updated["version"].(float64) != 2 {
		t.Fatalf("update: expected version 2, got %v", updated["version"])
	}

	// List versions
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/plans/"+planID+"/versions", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list versions: expected 200, got %d", resp.StatusCode)
	}

	var versionsResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&versionsResult)
	resp.Body.Close()

	versions := versionsResult["items"].([]interface{})
	if len(versions) != 2 {
		t.Fatalf("list versions: expected 2, got %d", len(versions))
	}

	// Get specific version
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/plans/"+planID+"/versions/1", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get version: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ = http.NewRequest("DELETE", ts.URL()+"/api/v1/plans/"+planID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/plans/"+planID, nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPlanCreateValidation(t *testing.T) {
	ts := testutil.NewTestServer(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "empty name",
			body:       `{"name":"","root":{"id":"r","type":"plan","name":"r","enabled":true,"properties":{},"children":[]}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "invalid json",
			body:       `{not valid`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/plans", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp := ts.Do(req)

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}

			var errResp struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
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

func TestPlanNotFound(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/plans/nonexistent-id", nil)
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

	if errResp.Error.Code != "PLAN_NOT_FOUND" {
		t.Fatalf("expected PLAN_NOT_FOUND, got %q", errResp.Error.Code)
	}
}

func TestPlanValidate(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a plan first
	body := `{"name":"Test","root":{"id":"r","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/plans", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	planID := created["id"].(string)

	// Validate with a good root
	validateBody := `{"root":{"id":"r","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/plans/%s/validate", ts.URL(), planID), bytes.NewBufferString(validateBody))
	req.Header.Set("Content-Type", "application/json")
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("validate: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result["valid"] != true {
		t.Fatalf("validate: expected valid=true, got %v", result["valid"])
	}
}
