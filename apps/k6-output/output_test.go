package output

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.k6.io/k6/metrics"
)

func samples(t *testing.T, now time.Time) []metrics.SampleContainer {
	t.Helper()
	r := metrics.NewRegistry()
	reqs := r.MustNewMetric("http_reqs", metrics.Counter)
	dur := r.MustNewMetric("http_req_duration", metrics.Trend, metrics.Time)
	failed := r.MustNewMetric("http_req_failed", metrics.Rate)
	checks := r.MustNewMetric("checks", metrics.Rate)
	vus := r.MustNewMetric("vus", metrics.Gauge)
	ok := r.RootTagSet().With("method", "GET").With("name", "GET /products").With("status", "200").With("scenario", "default")
	bad := ok.With("status", "503").With("error_code", "1503").With("error", "service unavailable")
	control := ok.With("scenario", controlScenario)
	check := r.RootTagSet().With("group", "::Buy").With("check", "status is 200").With("scenario", "default")
	s := func(m *metrics.Metric, tags *metrics.TagSet, at time.Time, v float64) metrics.Sample {
		return metrics.Sample{TimeSeries: metrics.TimeSeries{Metric: m, Tags: tags}, Time: at, Value: v}
	}
	return []metrics.SampleContainer{metrics.Samples{
		s(reqs, ok, now, 1), s(dur, ok, now, 10), s(failed, ok, now, 0), s(checks, check, now, 1),
		s(reqs, bad, now.Add(time.Second), 1), s(dur, bad, now.Add(time.Second), 30), s(failed, bad, now.Add(time.Second), 1), s(checks, check, now.Add(time.Second), 0),
		s(reqs, control, now, 1), s(dur, control, now, 999),
		s(vus, r.RootTagSet(), now, 3),
	}}
}

func TestFoldAggregatesPerSecondAndSkipsControlScenario(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	b := &Batch{}
	fold(b, samples(t, now))
	fold(b, samples(t, now)) // a second flush adds to the same seconds

	if len(b.Seconds) != 2 {
		t.Fatalf("seconds = %d, want 2", len(b.Seconds))
	}
	first, second := b.Seconds[0], b.Seconds[1]
	if first.Requests != 2 || second.Requests != 2 || second.Failed != 2 {
		t.Errorf("counts: first %+v, second %+v", first, second)
	}
	if first.Durations.Count != 2 || first.Durations.Max != 10 {
		t.Errorf("control-scenario samples leaked into durations: %+v", first.Durations)
	}
	if first.VUs == nil || *first.VUs != 3 {
		t.Errorf("vus = %v", first.VUs)
	}
	if len(b.Endpoints) != 1 || b.Endpoints[0].Failed != 2 || b.Endpoints[0].Durations.Count != 4 {
		t.Errorf("endpoints = %+v", b.Endpoints)
	}
	if len(b.Checks) != 1 || b.Checks[0].Passes != 2 || b.Checks[0].Fails != 2 {
		t.Errorf("checks = %+v", b.Checks)
	}
	if len(b.Errors) != 2 || b.Errors[0].Status != 503 || b.Errors[0].Message != "service unavailable" {
		t.Errorf("errors = %+v", b.Errors)
	}
	if BucketOf(10) != 461 { // round(ln(10) * 200): must match the control plane
		t.Errorf("BucketOf(10) = %d", BucketOf(10))
	}
}

func TestSendRetriesUntilAccepted(t *testing.T) {
	var mu sync.Mutex
	var got []Batch
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-OpenSynapse-Run-Token") != "tok" {
			t.Errorf("missing run token")
		}
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var b Batch
		json.NewDecoder(zr).Decode(&b)
		mu.Lock()
		defer mu.Unlock()
		if fail {
			fail = false
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		got = append(got, b)
	}))
	defer srv.Close()

	o := &Output{url: srv.URL, token: "tok", instance: "runner-1", client: srv.Client(), now: time.Now}
	o.AddMetricSamples(samples(t, time.Now()))
	o.flush() // rejected with 503: kept
	o.flush() // resent
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Instance != "runner-1" || len(got[0].Seconds) != 2 {
		t.Fatalf("delivered %+v", got)
	}
}
