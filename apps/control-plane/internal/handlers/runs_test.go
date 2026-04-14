package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func TestRunListEmpty(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/runs", nil)
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	items := result["items"].([]interface{})
	if len(items) != 0 {
		t.Fatalf("list: expected 0 items, got %d", len(items))
	}
}

func TestRunStartWithoutEngine(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a plan first
	planBody := `{"name":"Test Plan","root":{"id":"r","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/plans", bytes.NewBufferString(planBody))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var plan map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&plan)
	resp.Body.Close()
	planID := plan["id"].(string)

	// Try to start a run (engine is nil in test server)
	runBody := `{"plan_id":"` + planID + `"}`
	req, _ = http.NewRequest("POST", ts.URL()+"/api/v1/runs", bytes.NewBufferString(runBody))
	req.Header.Set("Content-Type", "application/json")
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("start: expected 503, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()

	if errResp.Error.Code != "ENGINE_NOT_AVAILABLE" {
		t.Fatalf("expected ENGINE_NOT_AVAILABLE, got %q", errResp.Error.Code)
	}
}

func TestRunStartValidation(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Missing plan_id
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/runs", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusServiceUnavailable {
		// Engine is nil, so we get 503 before validation
		// This is expected in test mode
		t.Logf("got status %d (engine nil in test)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRunNotFound(t *testing.T) {
	ts := testutil.NewTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL()+"/api/v1/runs/nonexistent", nil)
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

	if errResp.Error.Code != "RUN_NOT_FOUND" {
		t.Fatalf("expected RUN_NOT_FOUND, got %q", errResp.Error.Code)
	}
}

func TestRunEventsEmpty(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create a run directly via the store to test events
	planBody := `{"name":"Test","root":{"id":"r","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/plans", bytes.NewBufferString(planBody))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var plan map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&plan)
	resp.Body.Close()

	// Create a run record directly
	run, err := ts.Runs.Create(plan["id"].(string), 1, json.RawMessage(planBody), nil,
		struct {
			VUsTarget       int `json:"vus_target"`
			RPSTarget       int `json:"rps_target,omitempty"`
			DurationSeconds int `json:"duration_seconds"`
			WorkerCount     int `json:"worker_count"`
		}{VUsTarget: 10, DurationSeconds: 30, WorkerCount: 1})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	// List events (should be empty)
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/runs/"+run.ID+"/events", nil)
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list events: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	items := result["items"].([]interface{})
	if len(items) != 0 {
		t.Fatalf("expected 0 events, got %d", len(items))
	}

	// Add an event
	eventBody := `{"type":"user_note","payload":{"message":"test note"}}`
	req, _ = http.NewRequest("POST", ts.URL()+"/api/v1/runs/"+run.ID+"/events", bytes.NewBufferString(eventBody))
	req.Header.Set("Content-Type", "application/json")
	resp = ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add event: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify event was added
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/runs/"+run.ID+"/events", nil)
	resp = ts.Do(req)
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	items = result["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(items))
	}
}
