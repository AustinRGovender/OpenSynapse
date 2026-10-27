package engine

import (
	"bufio"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// testdata/k6-points.jsonl is real `k6 run --out json` output (k6 v1.8.2)
// from a run against the echo test API.
func loadFixture(t *testing.T) [][]byte {
	t.Helper()
	f, err := os.Open("testdata/k6-points.jsonl")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	var lines [][]byte
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for s.Scan() {
		lines = append(lines, append([]byte(nil), s.Bytes()...))
	}
	return lines
}

func TestAggregatorSummarisesRealK6Output(t *testing.T) {
	lines := loadFixture(t)
	a := newAggregator(0)
	var reqs int
	for _, l := range lines {
		a.ingest("", l)
		if strings.Contains(string(l), `"metric":"http_reqs"`) {
			reqs++
		}
	}
	if reqs == 0 {
		t.Fatal("fixture has no http_reqs points")
	}
	s := a.summary()
	if s.TotalRequests != int64(reqs) {
		t.Errorf("total requests = %d, want %d", s.TotalRequests, reqs)
	}
	if s.FailedRequests != 0 || s.ErrorRate != 0 {
		t.Errorf("echo /health never fails; got %d failed, %.2f%%", s.FailedRequests, s.ErrorRate)
	}
	if s.P95MS <= 0 || s.P95MS > s.MaxMS || s.P50MS > s.P95MS {
		t.Errorf("implausible percentiles: p50=%.3f p95=%.3f max=%.3f", s.P50MS, s.P95MS, s.MaxMS)
	}
	if s.AssertionPasses == 0 {
		t.Error("expected passing checks from the fixture")
	}
	if len(s.Endpoints) != 1 || s.Endpoints[0].Method != "GET" || !strings.Contains(s.Endpoints[0].Name, "/health") {
		t.Errorf("unexpected endpoint breakdown: %+v", s.Endpoints)
	}
	if len(s.Checks) != 1 || s.Checks[0].Name != "ok" {
		t.Errorf("unexpected checks breakdown: %+v", s.Checks)
	}
	if s.PeakVUs == 0 {
		t.Error("peak VUs should come from the vus gauge")
	}

	points := a.snapshots(time.Now().Unix())
	if len(points) == 0 {
		t.Fatal("expected per-second points")
	}
	var sum int64
	for _, p := range points {
		sum += p.Requests
	}
	if sum != s.TotalRequests {
		t.Errorf("per-second requests sum to %d, want %d", sum, s.TotalRequests)
	}
}

func TestAggregatorCountsFailuresAndSkipsControlScenario(t *testing.T) {
	a := newAggregator(1)
	ts := `"time":"2026-10-27T10:00:00.5Z"`
	ingest := func(metric, value, tags string) {
		a.ingest("", []byte(`{"type":"Point","metric":"`+metric+`","data":{`+ts+`,"value":`+value+`,"tags":{`+tags+`}}}`))
	}
	user := `"scenario":"main","method":"GET","name":"http://x/err","url":"http://x/err","status":"500"`
	control := `"scenario":"` + ControlScenario + `","method":"GET","name":"opensynapse-control","status":"200"`
	for i := 0; i < 4; i++ {
		ingest("http_reqs", "1", user)
		ingest("http_req_duration", "12", user)
		ingest("http_req_failed", "1", user)
	}
	ingest("http_reqs", "1", control)
	ingest("http_req_failed", "0", control)
	ingest("vus", "6", "")

	s := a.summary()
	if s.TotalRequests != 4 || s.FailedRequests != 4 || s.ErrorRate != 100 {
		t.Errorf("got requests=%d failed=%d rate=%.1f; control traffic must be excluded", s.TotalRequests, s.FailedRequests, s.ErrorRate)
	}
	if s.PeakVUs != 5 {
		t.Errorf("peak VUs = %d, want 5 (control VU hidden)", s.PeakVUs)
	}
	errs := a.takeErrors()
	if len(errs) != 4 || errs[0].Status != 500 || errs[0].URL != "http://x/err" {
		t.Errorf("unexpected error samples: %+v", errs)
	}
	if len(a.takeErrors()) != 0 {
		t.Error("takeErrors should drain")
	}
	pts := a.snapshots(time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC).Unix())
	if len(pts) != 1 || pts[0].ErrorRate != 100 || pts[0].RPS != 4 {
		t.Errorf("unexpected snapshot: %+v", pts)
	}
}

func TestSnapshotsFillQuietSeconds(t *testing.T) {
	a := newAggregator(0)
	a.ingest("", []byte(`{"type":"Point","metric":"http_reqs","data":{"time":"2026-10-27T10:00:00Z","value":1,"tags":{}}}`))
	a.ingest("", []byte(`{"type":"Point","metric":"http_reqs","data":{"time":"2026-10-27T10:00:03Z","value":1,"tags":{}}}`))
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC).Unix()
	pts := a.snapshots(base + 3)
	if len(pts) != 4 {
		t.Fatalf("expected 4 continuous points, got %d", len(pts))
	}
	if pts[1].Requests != 0 || pts[3].Requests != 1 {
		t.Errorf("unexpected fill: %+v", pts)
	}
	if more := a.snapshots(base + 3); len(more) != 0 {
		t.Error("seconds must only be emitted once")
	}
}

func TestHistogramPercentileAccuracy(t *testing.T) {
	h := newHistogram()
	r := rand.New(rand.NewPCG(1, 2))
	vals := make([]float64, 50000)
	for i := range vals {
		vals[i] = math.Exp(r.NormFloat64()) * 100 // log-normal latencies around 100ms
		h.add(vals[i])
	}
	sort.Float64s(vals)
	for _, q := range []float64{0.5, 0.9, 0.95, 0.99} {
		exact := exactPercentile(vals, q)
		got := h.percentile(q)
		if math.Abs(got-exact)/exact > 0.01 {
			t.Errorf("p%.0f = %.3f, exact %.3f (>1%% off)", q*100, got, exact)
		}
	}
	if h.percentile(1) != vals[len(vals)-1] {
		t.Error("p100 must equal the exact max")
	}
}

func TestScheduleTargetAt(t *testing.T) {
	s := Schedule{StartVUs: 0, Stages: []Stage{{Duration: 10 * time.Second, Target: 10}, {Duration: 10 * time.Second, Target: 10}, {Duration: 5 * time.Second, Target: 0}}}
	cases := map[time.Duration]int{0: 0, 5 * time.Second: 5, 10 * time.Second: 10, 15 * time.Second: 10, 22500 * time.Millisecond: 5, 30 * time.Second: 0}
	for at, want := range cases {
		if got := s.TargetAt(at); got != want {
			t.Errorf("TargetAt(%s) = %d, want %d", at, got, want)
		}
	}
	if s.Duration() != 25*time.Second || s.PeakVUs() != 10 {
		t.Errorf("duration=%s peak=%d", s.Duration(), s.PeakVUs())
	}
}

func TestParseCapabilities(t *testing.T) {
	out := "k6 v1.8.2 (go1.27.3, linux/amd64)\nExtensions:\n  github.com/oleiade/xk6-kv v1.3.0, k6/x/kv [js]\n"
	c := parseCapabilities(out)
	if c.Version != "v1.8.2" || !c.KV {
		t.Errorf("parsed %+v", c)
	}
	plain := parseCapabilities("k6.exe v1.7.1 (commit/9f82e6f1fc, go1.26.1, windows/amd64)\n")
	if plain.Version != "v1.7.1" || plain.KV {
		t.Errorf("parsed %+v", plain)
	}
	e := &Engine{caps: plain}
	if e.noSummaryFlag() != "--no-summary" {
		t.Error("k6 v1 uses --no-summary")
	}
	e.caps.Version = "v2.3.0"
	if e.noSummaryFlag() != "--summary-mode=disabled" {
		t.Error("k6 v2 uses --summary-mode=disabled")
	}
}

func TestFailureMessage(t *testing.T) {
	msg := failureMessage(107, []string{
		`time="x" level=info msg="starting"`,
		`time="x" level=error msg="ReferenceError: foo is not defined\n\tat default (file:///s.js:3:3(1))" source=stacktrace`,
	})
	if !strings.Contains(msg, "the script threw an exception (exit code 107)") || !strings.Contains(msg, "ReferenceError: foo is not defined") {
		t.Errorf("unexpected message: %s", msg)
	}
}

func TestBaseEnvDisablesPhoneHome(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://secret")
	env := strings.Join(baseEnv(), "\n")
	for _, want := range []string{"K6_NO_USAGE_REPORT=true", "K6_AUTO_EXTENSION_RESOLUTION=false"} {
		if !strings.Contains(env, want) {
			t.Errorf("baseEnv missing %s", want)
		}
	}
	if strings.Contains(env, "postgres://secret") {
		t.Error("control-plane secrets must not leak into the k6 environment")
	}
}
