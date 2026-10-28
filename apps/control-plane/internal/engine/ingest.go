package engine

import (
	"fmt"
	"math"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// Cluster workers aggregate their samples with the opensynapse k6 output
// (apps/k6-output) and post one batch a second to
// POST /api/v1/runs/{id}/ingest. These types mirror that extension's.

// IngestBatch is one worker's report.
type IngestBatch struct {
	Instance  string           `json:"instance"`
	Through   time.Time        `json:"through"`
	Seconds   []IngestSecond   `json:"seconds,omitempty"`
	Endpoints []IngestEndpoint `json:"endpoints,omitempty"`
	Checks    []IngestCheck    `json:"checks,omitempty"`
	Errors    []db.RunError    `json:"errors,omitempty"`
	Final     bool             `json:"final,omitempty"`
}

// IngestSecond holds one second's counts from one worker.
type IngestSecond struct {
	Unix          int64       `json:"t"`
	Requests      int64       `json:"requests,omitempty"`
	Failed        int64       `json:"failed,omitempty"`
	Iterations    int64       `json:"iterations,omitempty"`
	ChecksPassed  int64       `json:"checks_passed,omitempty"`
	ChecksFailed  int64       `json:"checks_failed,omitempty"`
	BytesSent     float64     `json:"bytes_sent,omitempty"`
	BytesReceived float64     `json:"bytes_received,omitempty"`
	VUs           *int64      `json:"vus,omitempty"`
	Durations     *IngestHist `json:"durations,omitempty"`
}

// IngestHist is a histogram in the same buckets as histogram.
type IngestHist struct {
	Buckets map[int32]int64 `json:"b"`
	Count   int64           `json:"n"`
	Sum     float64         `json:"s"`
	Min     float64         `json:"min"`
	Max     float64         `json:"max"`
}

// IngestEndpoint and IngestCheck are deltas since the worker's previous batch.
type IngestEndpoint struct {
	Method    string      `json:"method"`
	Name      string      `json:"name"`
	Failed    int64       `json:"failed,omitempty"`
	Durations *IngestHist `json:"durations,omitempty"`
}

type IngestCheck struct {
	Group  string `json:"group"`
	Name   string `json:"name"`
	Passes int64  `json:"passes,omitempty"`
	Fails  int64  `json:"fails,omitempty"`
}

// mergeInto adds a worker's histogram to h.
func (in *IngestHist) mergeInto(h *histogram) {
	if in == nil || in.Count == 0 {
		return
	}
	for b, n := range in.Buckets {
		h.buckets[b] += n
	}
	h.count += in.Count
	h.sum += in.Sum
	h.min = math.Min(h.min, in.Min)
	h.max = math.Max(h.max, in.Max)
}

// merge folds a worker's batch into the run's aggregates.
func (a *aggregator) merge(b *IngestBatch) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, s := range b.Seconds {
		at := time.Unix(s.Unix, 0)
		if a.firstTS.IsZero() || at.Before(a.firstTS) {
			a.firstTS = at
		}
		if end := at.Add(time.Second); end.After(a.lastTS) {
			a.lastTS = end
		}
		bk := a.bucket(s.Unix)
		bk.requests += s.Requests
		bk.failed += s.Failed
		bk.iterations += s.Iterations
		bk.checksPassed += s.ChecksPassed
		bk.checksFailed += s.ChecksFailed
		bk.bytesSent += s.BytesSent
		bk.bytesRecv += s.BytesReceived
		a.requests += s.Requests
		a.failed += s.Failed
		a.iterations += s.Iterations
		a.checksPass += s.ChecksPassed
		a.checksFail += s.ChecksFailed
		a.bytesSent += s.BytesSent
		a.bytesRecv += s.BytesReceived
		if s.Durations != nil {
			if bk.hist == nil {
				bk.hist = newHistogram()
			}
			s.Durations.mergeInto(bk.hist)
			s.Durations.mergeInto(a.total)
		}
		if s.VUs != nil {
			a.setInstanceVUs(b.Instance, int(*s.VUs), bk)
		}
	}
	for _, e := range b.Endpoints {
		agg := a.endpoint(map[string]string{"method": e.Method, "name": e.Name})
		agg.failed += e.Failed
		e.Durations.mergeInto(agg.hist)
	}
	for _, c := range b.Checks {
		agg := a.check(map[string]string{"group": c.Group, "check": c.Name})
		agg.passes += c.Passes
		agg.fails += c.Fails
	}
	for _, e := range b.Errors {
		a.errorsTotal++
		a.errors = append(a.errors, e)
		if len(a.errors) > maxErrorSamples {
			a.errors = a.errors[len(a.errors)-maxErrorSamples:]
		}
		if len(a.newErrors) < maxErrorSamples {
			a.newErrors = append(a.newErrors, e)
		}
	}
	if b.Final {
		delete(a.latest, b.Instance)
	} else if b.Through.After(a.latest[b.Instance]) {
		a.latest[b.Instance] = b.Through
	}
}

// Ingest takes a batch from one of a run's workers.
func (e *Engine) Ingest(runID, token string, b *IngestBatch) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	if rp.spec.ControlToken == "" || token != rp.spec.ControlToken {
		return fmt.Errorf("invalid run token")
	}
	if b.Instance == "" {
		return fmt.Errorf("batch has no instance")
	}
	rp.agg.merge(b)
	return nil
}
