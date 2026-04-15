package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
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
	Runs    []db.Run          `json:"runs"`
	Changes []metricChange    `json:"changes"`
}

type metricChange struct {
	Metric     string  `json:"metric"`
	RunAID     string  `json:"run_a_id"`
	RunBID     string  `json:"run_b_id"`
	ValueA     float64 `json:"value_a"`
	ValueB     float64 `json:"value_b"`
	AbsDiff    float64 `json:"abs_diff"`
	PctChange  float64 `json:"pct_change"`
	Improved   bool    `json:"improved"`
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

	// Load all runs
	var runs []db.Run
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
		runs = append(runs, *run)
	}

	// Compute metric changes between first and last run
	changes := computeChanges(runs)

	writeJSON(w, http.StatusOK, comparisonResult{
		Runs:    runs,
		Changes: changes,
	})
}

func computeChanges(runs []db.Run) []metricChange {
	if len(runs) < 2 {
		return nil
	}

	first := runs[0]
	last := runs[len(runs)-1]

	if first.Summary == nil || last.Summary == nil {
		return nil
	}

	a := first.Summary
	b := last.Summary

	metrics := []struct {
		name     string
		valueA   float64
		valueB   float64
		lowerBetter bool
	}{
		{"p95_ms", a.P95MS, b.P95MS, true},
		{"p90_ms", a.P90MS, b.P90MS, true},
		{"p50_ms", a.P50MS, b.P50MS, true},
		{"error_rate", a.ErrorRate, b.ErrorRate, true},
		{"throughput_rps", a.ThroughputRPS, b.ThroughputRPS, false},
	}

	var changes []metricChange
	for _, m := range metrics {
		absDiff := m.valueB - m.valueA
		pctChange := 0.0
		if m.valueA != 0 {
			pctChange = (absDiff / m.valueA) * 100
		}

		// Only flag significant changes (>5%)
		if abs(pctChange) < 5 {
			continue
		}

		improved := absDiff < 0
		if !m.lowerBetter {
			improved = absDiff > 0
		}

		changes = append(changes, metricChange{
			Metric:    m.name,
			RunAID:    first.ID,
			RunBID:    last.ID,
			ValueA:    m.valueA,
			ValueB:    m.valueB,
			AbsDiff:   absDiff,
			PctChange: pctChange,
			Improved:  improved,
		})
	}

	return changes
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
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
