package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func createTestRunWithSummary(t *testing.T, ts *testutil.TestServer, name string, p95 float64, rps float64) string {
	t.Helper()
	// Create a plan
	planBody := `{"name":"` + name + `","root":{"id":"r","type":"plan","name":"root","enabled":true,"properties":{},"children":[]}}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/plans", bytes.NewBufferString(planBody))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	var plan map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&plan)
	resp.Body.Close()

	// Create a run directly
	run, err := ts.Runs.Create(plan["id"].(string), 1, json.RawMessage(planBody), nil,
		db.RunParameters{VUsTarget: 10, DurationSeconds: 30, WorkerCount: 1})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	ts.Runs.UpdateStatus(run.ID, "completed")
	ts.Runs.UpdateSummary(run.ID, &db.RunSummary{
		TotalRequests: 1000,
		FailedRequests: 5,
		ErrorRate:     0.005,
		ThroughputRPS: rps,
		P50MS:         p95 * 0.7,
		P90MS:         p95 * 0.9,
		P95MS:         p95,
		P99MS:         p95 * 1.2,
		MaxMS:         p95 * 2,
		ThresholdsPassed: true,
	})

	return run.ID
}

func TestCompareRuns(t *testing.T) {
	ts := testutil.NewTestServer(t)

	run1 := createTestRunWithSummary(t, ts, "Run 1", 200.0, 100.0)
	run2 := createTestRunWithSummary(t, ts, "Run 2", 150.0, 120.0)

	body := `{"run_ids":["` + run1 + `","` + run2 + `"],"metrics":["p95_ms","throughput_rps"]}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/compare", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("compare: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	runs := result["runs"].([]interface{})
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	changes := result["changes"].([]interface{})
	if len(changes) == 0 {
		t.Fatal("expected at least one metric change")
	}

	// p95 improved (200→150, -25%)
	foundP95 := false
	for _, c := range changes {
		ch := c.(map[string]interface{})
		if ch["metric"] == "p95_ms" {
			foundP95 = true
			if ch["improved"] != true {
				t.Fatal("expected p95 improvement")
			}
		}
	}
	if !foundP95 {
		t.Fatal("expected p95_ms in changes")
	}
}

func TestCompareMinimumRuns(t *testing.T) {
	ts := testutil.NewTestServer(t)

	body := `{"run_ids":["one"],"metrics":[]}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/compare", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestReportCRUD(t *testing.T) {
	ts := testutil.NewTestServer(t)

	// Create
	body := `{"name":"Regression Check","run_ids":["a","b"],"metrics":["p95_ms"],"normalisation":"elapsed_time"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/reports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}

	var created map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	reportID := created["id"].(string)
	if created["name"] != "Regression Check" {
		t.Fatalf("expected name 'Regression Check', got %v", created["name"])
	}

	// Get
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/reports/"+reportID, nil)
	resp = ts.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	req, _ = http.NewRequest("GET", ts.URL()+"/api/v1/reports", nil)
	resp = ts.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var listResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResult)
	resp.Body.Close()
	items := listResult["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 report, got %d", len(items))
	}

	// Delete
	req, _ = http.NewRequest("DELETE", ts.URL()+"/api/v1/reports/"+reportID, nil)
	resp = ts.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}
}

func TestExportJSON(t *testing.T) {
	ts := testutil.NewTestServer(t)

	runID := createTestRunWithSummary(t, ts, "Export Test", 100.0, 50.0)

	body := `{"format":"json"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/runs/"+runID+"/export", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export json: expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	cd := resp.Header.Get("Content-Disposition")
	if cd == "" {
		t.Fatal("expected Content-Disposition header")
	}
	resp.Body.Close()
}

func TestExportCSV(t *testing.T) {
	ts := testutil.NewTestServer(t)

	runID := createTestRunWithSummary(t, ts, "CSV Test", 100.0, 50.0)

	body := `{"format":"csv"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/runs/"+runID+"/export", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export csv: expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/csv" {
		t.Fatalf("expected text/csv, got %s", ct)
	}
	resp.Body.Close()
}

func TestExportHTML(t *testing.T) {
	ts := testutil.NewTestServer(t)

	runID := createTestRunWithSummary(t, ts, "HTML Test", 100.0, 50.0)

	body := `{"format":"html"}`
	req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/runs/"+runID+"/export", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export html: expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/html" {
		t.Fatalf("expected text/html, got %s", ct)
	}
	resp.Body.Close()
}
