package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/stats"
)

type ReportHandlers struct {
	reports *db.ReportStore
	runs    *db.RunStore
}

func NewReportHandlers(reports *db.ReportStore, runs *db.RunStore) *ReportHandlers {
	return &ReportHandlers{reports: reports, runs: runs}
}

// Compare computes a comparison between multiple runs.
type compareRequest struct {
	RunIDs  []string `json:"run_ids"`
	Metrics []string `json:"metrics"`
}

type comparisonResult struct {
	Runs    []db.Run       `json:"runs"`
	Changes []metricChange `json:"changes"`
}

// metricChange compares one metric between the baseline run (A, the first
// selected) and another run (B).
type metricChange struct {
	Metric      string  `json:"metric"`
	RunAID      string  `json:"run_a_id"`
	RunBID      string  `json:"run_b_id"`
	ValueA      float64 `json:"value_a"`
	ValueB      float64 `json:"value_b"`
	AbsDiff     float64 `json:"abs_diff"`
	PctChange   float64 `json:"pct_change"`
	Improved    bool    `json:"improved"`
	Significant bool    `json:"significant"`
	PValue      float64 `json:"p_value"`
}

// Changes smaller than this are never flagged, however consistent.
const minPracticalChangePct = 2.0

// significance is the p-value below which a difference is called real.
const significance = 0.05

type comparedMetric struct {
	name        string
	lowerBetter bool
	summary     func(s *db.RunSummary) float64
	perSecond   func(p db.MetricPoint) float64
}

var comparedMetrics = []comparedMetric{
	{"p95_ms", true, func(s *db.RunSummary) float64 { return s.P95MS }, func(p db.MetricPoint) float64 { return p.P95MS }},
	{"p50_ms", true, func(s *db.RunSummary) float64 { return s.P50MS }, func(p db.MetricPoint) float64 { return p.P50MS }},
	{"error_rate", true, func(s *db.RunSummary) float64 { return s.ErrorRate }, func(p db.MetricPoint) float64 { return p.ErrorRate }},
	{"throughput_rps", false, func(s *db.RunSummary) float64 { return s.ThroughputRPS }, func(p db.MetricPoint) float64 { return p.RPS }},
}

func (h *ReportHandlers) Compare(w http.ResponseWriter, r *http.Request) {
	var req compareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if len(req.RunIDs) < 2 {
		badRequest(w, "VALIDATION_ERROR", "At least 2 run IDs are required", nil)
		return
	}
	if len(req.RunIDs) > 5 {
		badRequest(w, "VALIDATION_ERROR", "At most 5 run IDs are supported", nil)
		return
	}

	var runs []db.Run
	series := map[string][]db.MetricPoint{}
	for _, id := range req.RunIDs {
		run, err := h.runs.Get(id)
		if err != nil {
			internalError(w, err)
			return
		}
		if run == nil {
			notFound(w, "RUN", id)
			return
		}
		pts, err := h.runs.ListMetrics(id, 0, 0)
		if err != nil {
			internalError(w, err)
			return
		}
		run.PlanSnapshot = nil
		runs = append(runs, *run)
		series[id] = pts
	}

	writeJSON(w, http.StatusOK, comparisonResult{Runs: runs, Changes: computeChanges(runs, series)})
}

// activeSeconds keeps seconds in which requests completed, so ramp-up gaps
// and pauses don't dilute the comparison.
func activeSeconds(pts []db.MetricPoint, f func(db.MetricPoint) float64) []float64 {
	var out []float64
	for _, p := range pts {
		if p.Requests > 0 {
			out = append(out, f(p))
		}
	}
	return out
}

// computeChanges compares every run against the first (baseline). A change
// is significant when Welch's t-test on the per-second values gives
// p < 0.05 and the change is at least 2%; runs too short for the test fall
// back to a 5% threshold on the summary values.
func computeChanges(runs []db.Run, series map[string][]db.MetricPoint) []metricChange {
	if len(runs) < 2 || runs[0].Summary == nil {
		return []metricChange{}
	}
	base := runs[0]
	changes := []metricChange{}
	for _, other := range runs[1:] {
		if other.Summary == nil {
			continue
		}
		for _, m := range comparedMetrics {
			a, b := m.summary(base.Summary), m.summary(other.Summary)
			diff := b - a
			pct := 0.0
			if a != 0 {
				pct = diff / a * 100
			} else if b != 0 {
				pct = 100
			}

			sa := activeSeconds(series[base.ID], m.perSecond)
			sb := activeSeconds(series[other.ID], m.perSecond)
			p := 1.0
			significant := false
			if len(sa) >= 5 && len(sb) >= 5 {
				p = stats.WelchTTest(sa, sb)
				significant = p < significance && math.Abs(pct) >= minPracticalChangePct
			} else {
				significant = math.Abs(pct) >= 5
			}

			improved := diff < 0
			if !m.lowerBetter {
				improved = diff > 0
			}
			changes = append(changes, metricChange{
				Metric: m.name, RunAID: base.ID, RunBID: other.ID,
				ValueA: a, ValueB: b, AbsDiff: diff, PctChange: pct,
				Improved: improved, Significant: significant, PValue: p,
			})
		}
	}
	return changes
}

// Reports CRUD

func (h *ReportHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, err := h.reports.List(db.ListParams{Limit: limit})
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ReportHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string   `json:"name"`
		RunIDs        []string `json:"run_ids"`
		Metrics       []string `json:"metrics"`
		Normalisation string   `json:"normalisation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	report, err := h.reports.Create(req.Name, req.RunIDs, req.Metrics, req.Normalisation)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, report)
}

func (h *ReportHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	report, err := h.reports.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if report == nil {
		notFound(w, "REPORT", id)
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func (h *ReportHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.reports.Delete(id); err != nil {
		notFound(w, "REPORT", id)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
