package engine

import (
	"math"
	"testing"
	"time"
)

func hist(values ...float64) *IngestHist {
	h := newHistogram()
	for _, v := range values {
		h.add(v)
	}
	return &IngestHist{Buckets: h.buckets, Count: h.count, Sum: h.sum, Min: h.min, Max: h.max}
}

// Two workers' batches combine exactly as if one process had seen every sample.
func TestMergeCombinesWorkersExactly(t *testing.T) {
	a := newAggregator(1) // one control VU per worker
	sec := time.Now().Truncate(time.Second).Unix() - 10
	three, four := int64(3), int64(4)
	a.merge(&IngestBatch{Instance: "w1", Through: time.Unix(sec+1, 0), Seconds: []IngestSecond{{Unix: sec, Requests: 2, Failed: 1, VUs: &three, Durations: hist(10, 20)}},
		Endpoints: []IngestEndpoint{{Method: "GET", Name: "/a", Failed: 1, Durations: hist(10, 20)}}, Checks: []IngestCheck{{Group: "", Name: "ok", Passes: 1, Fails: 1}}})
	a.merge(&IngestBatch{Instance: "w2", Through: time.Unix(sec+1, 0), Seconds: []IngestSecond{{Unix: sec, Requests: 2, VUs: &four, Durations: hist(30, 40)}},
		Endpoints: []IngestEndpoint{{Method: "GET", Name: "/a", Durations: hist(30, 40)}}, Checks: []IngestCheck{{Group: "", Name: "ok", Passes: 2}}})

	ref := newAggregator(0)
	for _, v := range []float64{10, 20, 30, 40} {
		ref.total.add(v)
	}
	s := a.summary()
	if s.TotalRequests != 4 || s.FailedRequests != 1 || s.PeakVUs != 5 {
		t.Fatalf("summary = %+v", s)
	}
	if math.Abs(s.P95MS-ref.total.percentile(0.95)) > 1e-9 || s.MaxMS != 40 {
		t.Errorf("p95 = %v (want %v), max = %v", s.P95MS, ref.total.percentile(0.95), s.MaxMS)
	}
	if len(s.Endpoints) != 1 || s.Endpoints[0].Requests != 4 || s.Endpoints[0].Failed != 1 {
		t.Errorf("endpoints = %+v", s.Endpoints)
	}
	if len(s.Checks) != 1 || s.Checks[0].Passes != 3 || s.Checks[0].Fails != 1 {
		t.Errorf("checks = %+v", s.Checks)
	}
	pts := a.snapshots(sec)
	if len(pts) != 1 || pts[0].Requests != 4 || pts[0].ActiveVUs != 5 || pts[0].P50MS < 19 || pts[0].P50MS > 21 {
		t.Errorf("per-second point = %+v", pts)
	}
	if wm, ok := a.watermark(); !ok || wm != sec+1 {
		t.Errorf("watermark = %d, %v", wm, ok)
	}
	a.merge(&IngestBatch{Instance: "w1", Final: true})
	a.merge(&IngestBatch{Instance: "w2", Final: true})
	if _, ok := a.watermark(); ok {
		t.Error("finished workers still hold the watermark")
	}
}

func TestIngestNeedsTheRunToken(t *testing.T) {
	e := &Engine{active: map[string]*runProcess{"r": {spec: RunSpec{ControlToken: "secret"}, agg: newAggregator(0)}}}
	if err := e.Ingest("r", "wrong", &IngestBatch{Instance: "w"}); err == nil {
		t.Error("accepted a batch with the wrong token")
	}
	if err := e.Ingest("r", "secret", &IngestBatch{Instance: "w"}); err != nil {
		t.Errorf("rejected a valid batch: %v", err)
	}
	if err := e.Ingest("missing", "secret", &IngestBatch{Instance: "w"}); err != ErrNotActive {
		t.Errorf("unknown run: %v", err)
	}
}
