package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

func doJSON(t *testing.T, ts *testutil.TestServer, method, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL()+path, r)
	req.Header.Set("Content-Type", "application/json")
	resp := ts.Do(req)
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func createPlan(t *testing.T, ts *testutil.TestServer, root map[string]any) string {
	name, _ := root["name"].(string)
	t.Helper()
	resp, out := doJSON(t, ts, "POST", "/api/v1/plans", map[string]any{"name": name, "root": root})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create plan: %d %v", resp.StatusCode, out)
	}
	return out["id"].(string)
}

// TestRunLifecycleWithRealK6 runs a short plan through the real k6 binary
// against a local target and checks that metrics, summary, events, errors,
// and exports all reflect what happened. Skipped when k6 is not installed.
func TestRunLifecycleWithRealK6(t *testing.T) {
	if _, err := exec.LookPath("k6"); err != nil {
		t.Skip("k6 not installed")
	}
	eng, err := engine.New()
	if err != nil {
		t.Skip("k6 engine unavailable: ", err)
	}

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/fail" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer target.Close()

	ts := testutil.NewTestServerWith(t, testutil.Options{Engine: eng, DataDir: t.TempDir()})
	env, _ := ts.Envs.Create("target", map[string]db.Variable{"TARGET": {Value: target.URL}})

	planID := createPlan(t, ts, map[string]any{
		"id": "root", "type": "plan", "name": "root", "enabled": true, "properties": map[string]any{},
		"children": []any{map[string]any{
			"id": "s", "type": "scenario", "name": "Main", "enabled": true,
			"properties": map[string]any{"executor": "constant-vus", "vus": 1, "duration": "60s"},
			"children": []any{
				map[string]any{"id": "ok", "type": "http", "name": "ok", "enabled": true,
					"properties": map[string]any{"method": "GET", "url": "${TARGET}/ok", "headers": []any{}, "body": map[string]any{"type": "none"}},
					"children": []any{map[string]any{"id": "a", "type": "assertion", "name": "is 200", "enabled": true,
						"properties": map[string]any{"target": "status", "condition": "equals", "value": 200}, "children": []any{}}}},
				map[string]any{"id": "bad", "type": "http", "name": "bad", "enabled": true,
					"properties": map[string]any{"method": "GET", "url": "${TARGET}/fail", "headers": []any{}, "body": map[string]any{"type": "none"}},
					"children": []any{}},
				map[string]any{"id": "t", "type": "timer", "name": "think", "enabled": true,
					"properties": map[string]any{"timer_type": "constant", "duration_ms": 100}, "children": []any{}},
			},
		}},
	})

	// Override to 2 VUs for 4 seconds: the plan says 1 VU for 60s.
	resp, run := doJSON(t, ts, "POST", "/api/v1/runs", map[string]any{
		"plan_id": planID, "environment_id": env.ID,
		"parameters": map[string]any{"vus_target": 2, "duration_seconds": 4},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start run: %d %v", resp.StatusCode, run)
	}
	runID := run["id"].(string)

	// Live state is available while running; RPS control needs xk6-kv.
	time.Sleep(1500 * time.Millisecond)
	if resp, st := doJSON(t, ts, "GET", "/api/v1/runs/"+runID+"/live", nil); resp.StatusCode == http.StatusOK {
		if st["can_control_vus"] != true {
			t.Errorf("VU control should be available for a constant-vus plan: %v", st)
		}
	}
	resp, ctl := doJSON(t, ts, "PATCH", "/api/v1/runs/"+runID+"/control", map[string]any{"vus": 3})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("VU change should apply: %d %v", resp.StatusCode, ctl)
	}

	deadline := time.Now().Add(60 * time.Second)
	var final map[string]any
	for time.Now().Before(deadline) {
		_, final = doJSON(t, ts, "GET", "/api/v1/runs/"+runID, nil)
		if s := final["status"]; s != "running" && s != "queued" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if final["status"] != "completed" {
		t.Fatalf("run should complete, got %v", final["status"])
	}
	sum := final["summary"].(map[string]any)
	total := sum["total_requests"].(float64)
	failed := sum["failed_requests"].(float64)
	if total < 4 || failed < 1 || failed > total {
		t.Fatalf("implausible totals: %v", sum)
	}
	if rate := sum["error_rate"].(float64); rate < 30 || rate > 70 {
		t.Errorf("half the requests fail; error_rate=%.1f%%", rate)
	}
	if int64(total) > hits.Load() {
		t.Errorf("k6 reported %v requests but the target saw %d", total, hits.Load())
	}
	if len(sum["endpoints"].([]any)) != 2 || len(sum["checks"].([]any)) != 1 {
		t.Errorf("expected 2 endpoints and 1 check: %v", sum)
	}

	// The 4-second override was honoured (not the plan's 60 seconds).
	started, _ := time.Parse(time.RFC3339Nano, final["started_at"].(string))
	ended, _ := time.Parse(time.RFC3339Nano, final["ended_at"].(string))
	if d := ended.Sub(started); d > 20*time.Second {
		t.Errorf("run took %s; duration override ignored", d)
	}

	_, samples := doJSON(t, ts, "GET", "/api/v1/runs/"+runID+"/samples", nil)
	if n := len(samples["items"].([]any)); n < 2 {
		t.Errorf("expected per-second samples, got %d", n)
	}
	_, errs := doJSON(t, ts, "GET", "/api/v1/runs/"+runID+"/errors", nil)
	if n := len(errs["items"].([]any)); n == 0 {
		t.Error("expected recorded errors for /fail")
	}
	_, events := doJSON(t, ts, "GET", "/api/v1/runs/"+runID+"/events", nil)
	types := []string{}
	for _, e := range events["items"].([]any) {
		types = append(types, e.(map[string]any)["type"].(string))
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "start") || !strings.Contains(joined, "vu_change") || !strings.Contains(joined, "stop") {
		t.Errorf("unexpected events: %s", joined)
	}

	for _, format := range []string{"json", "csv", "html"} {
		req, _ := http.NewRequest("POST", ts.URL()+"/api/v1/runs/"+runID+"/export", strings.NewReader(`{"format":"`+format+`"}`))
		req.Header.Set("Content-Type", "application/json")
		resp := ts.Do(req)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || len(body) < 100 {
			t.Errorf("export %s: status %d, %d bytes", format, resp.StatusCode, len(body))
		}
		if format == "csv" && !strings.HasPrefix(string(body), "timestamp,metric,value,tags") {
			t.Errorf("csv export should contain raw samples, got %q", string(body[:60]))
		}
		if format == "html" && !strings.Contains(string(body), "<svg") {
			t.Error("html export should contain inline charts")
		}
	}
}

func TestRunStartRejectsInvalidOverrides(t *testing.T) {
	eng, err := engine.New()
	if err != nil {
		t.Skip("k6 not installed")
	}
	ts := testutil.NewTestServerWith(t, testutil.Options{Engine: eng})
	planID := createPlan(t, ts, map[string]any{"id": "r", "type": "plan", "name": "r", "enabled": true, "properties": map[string]any{}, "children": []any{}})
	for _, params := range []map[string]any{
		{"vus_target": -1},
		{"duration_seconds": -5},
		{"stages": []any{map[string]any{"duration": "soon", "target": 5}}},
	} {
		resp, out := doJSON(t, ts, "POST", "/api/v1/runs", map[string]any{"plan_id": planID, "parameters": params})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("params %v: expected 400, got %d %v", params, resp.StatusCode, out)
		}
	}
}

func TestPlanWithRunsCanBeDeleted(t *testing.T) {
	ts := testutil.NewTestServer(t)
	planID := createPlan(t, ts, map[string]any{"id": "r", "type": "plan", "name": "Doomed", "enabled": true, "properties": map[string]any{}, "children": []any{}})
	run, err := ts.Runs.Create(planID, 1, json.RawMessage(`{"name":"Doomed"}`), nil, db.RunParameters{VUsTarget: 1})
	if err != nil {
		t.Fatal(err)
	}
	resp, out := doJSON(t, ts, "DELETE", "/api/v1/plans/"+planID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete plan with runs: %d %v", resp.StatusCode, out)
	}
	_, got := doJSON(t, ts, "GET", "/api/v1/runs/"+run.ID, nil)
	if got["plan_name"] != "Doomed" {
		t.Errorf("run should keep the plan name from its snapshot, got %v", got["plan_name"])
	}
}

func TestMarkInterrupted(t *testing.T) {
	ts := testutil.NewTestServer(t)
	planID := createPlan(t, ts, map[string]any{"id": "r", "type": "plan", "name": "p", "enabled": true, "properties": map[string]any{}, "children": []any{}})
	run, _ := ts.Runs.Create(planID, 1, json.RawMessage(`{}`), nil, db.RunParameters{})
	ts.Runs.UpdateStatus(run.ID, "running")
	if n, err := ts.Runs.MarkInterrupted(); err != nil || n != 1 {
		t.Fatalf("MarkInterrupted = %d, %v", n, err)
	}
	got, _ := ts.Runs.Get(run.ID)
	if got.Status != "failed" || got.EndedAt == nil {
		t.Errorf("interrupted run should be failed with an end time: %+v", got)
	}
}

func TestRunListIncludesPlanNameAndFilters(t *testing.T) {
	ts := testutil.NewTestServer(t)
	planID := createPlan(t, ts, map[string]any{"id": "r", "type": "plan", "name": "Named Plan", "enabled": true, "properties": map[string]any{}, "children": []any{}})
	a, _ := ts.Runs.Create(planID, 1, json.RawMessage(`{}`), nil, db.RunParameters{})
	b, _ := ts.Runs.Create(planID, 1, json.RawMessage(`{}`), nil, db.RunParameters{})
	ts.Runs.UpdateStatus(a.ID, "completed")
	ts.Runs.UpdateStatus(b.ID, "failed")

	_, out := doJSON(t, ts, "GET", "/api/v1/runs?status=failed", nil)
	items := out["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != b.ID {
		t.Fatalf("status filter: %v", items)
	}
	if items[0].(map[string]any)["plan_name"] != "Named Plan" {
		t.Errorf("plan_name missing: %v", items[0])
	}
	_, out = doJSON(t, ts, "GET", "/api/v1/runs?plan_id="+planID, nil)
	if len(out["items"].([]any)) != 2 {
		t.Errorf("plan filter: %v", out["items"])
	}
}

func TestRunSamplesNotFound(t *testing.T) {
	ts := testutil.NewTestServer(t)
	resp, _ := doJSON(t, ts, "GET", "/api/v1/runs/nope/samples", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestControlStateRequiresToken(t *testing.T) {
	ts := testutil.NewTestServer(t)
	resp, _ := doJSON(t, ts, "GET", "/api/v1/runs/x/control-state", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("without an engine expected 503, got %d", resp.StatusCode)
	}
}
