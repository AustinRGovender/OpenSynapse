package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/compiler"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

type RunHandlers struct {
	runs   *db.RunStore
	plans  *db.PlanStore
	engine *engine.Engine
	ws     *wsserver.Server
}

func NewRunHandlers(runs *db.RunStore, plans *db.PlanStore, eng *engine.Engine, ws *wsserver.Server) *RunHandlers {
	return &RunHandlers{runs: runs, plans: plans, engine: eng, ws: ws}
}

type startRunRequest struct {
	PlanID        string           `json:"plan_id"`
	EnvironmentID *string          `json:"environment_id"`
	Parameters    *db.RunParameters `json:"parameters"`
}

func (h *RunHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cursor := r.URL.Query().Get("cursor")

	result, err := h.runs.List(db.ListParams{Limit: limit, Cursor: cursor})
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *RunHandlers) Start(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE",
			"k6 engine is not available. Install k6 to run tests.", nil)
		return
	}

	var req startRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.PlanID == "" {
		badRequest(w, "VALIDATION_ERROR", "plan_id is required", nil)
		return
	}

	// Get the plan
	plan, err := h.plans.Get(req.PlanID)
	if err != nil {
		internalError(w, err)
		return
	}
	if plan == nil {
		notFound(w, "PLAN", req.PlanID)
		return
	}

	// Default parameters
	params := db.RunParameters{
		VUsTarget:       10,
		DurationSeconds: 30,
		WorkerCount:     1,
	}
	if req.Parameters != nil {
		if req.Parameters.VUsTarget > 0 {
			params.VUsTarget = req.Parameters.VUsTarget
		}
		if req.Parameters.DurationSeconds > 0 {
			params.DurationSeconds = req.Parameters.DurationSeconds
		}
		if req.Parameters.RPSTarget > 0 {
			params.RPSTarget = req.Parameters.RPSTarget
		}
	}

	// Snapshot the plan
	planJSON, _ := json.Marshal(plan)

	// Create the run record
	run, err := h.runs.Create(plan.ID, plan.Version, planJSON, nil, params)
	if err != nil {
		internalError(w, err)
		return
	}

	// Compile the plan to k6 script
	script, err := compiler.Compile(plan)
	if err != nil {
		h.runs.UpdateStatus(run.ID, "failed")
		writeError(w, http.StatusUnprocessableEntity, "COMPILATION_FAILED", err.Error(), nil)
		return
	}

	// Update status to running
	h.runs.UpdateStatus(run.ID, "running")
	h.runs.AddEvent(run.ID, "start", nil)

	// Broadcast run started
	if h.ws != nil {
		h.ws.BroadcastRunEvent(run.ID, map[string]string{"type": "start"})
	}

	// Start k6 subprocess
	err = h.engine.StartRun(
		run.ID,
		script,
		params,
		// Metrics callback: broadcast to WS clients
		func(runID string, snapshot engine.MetricSnapshot) {
			if h.ws != nil {
				h.ws.BroadcastRunMetrics(runID, snapshot)
			}
		},
		// Completion callback
		func(runID string, exitCode int, summary *db.RunSummary) {
			status := "completed"
			if exitCode != 0 {
				status = "failed"
			}
			h.runs.UpdateStatus(runID, status)
			if summary != nil {
				h.runs.UpdateSummary(runID, summary)
			}
			h.runs.AddEvent(runID, "stop", nil)

			if h.ws != nil {
				h.ws.BroadcastRunEvent(runID, map[string]interface{}{
					"type":   "completed",
					"status": status,
				})
			}
		},
	)
	if err != nil {
		h.runs.UpdateStatus(run.ID, "failed")
		writeError(w, http.StatusInternalServerError, "ENGINE_ERROR", "Failed to start k6: "+err.Error(), nil)
		return
	}

	// Re-fetch with updated status
	run, _ = h.runs.Get(run.ID)
	writeJSON(w, http.StatusCreated, run)
}

func (h *RunHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	run, err := h.runs.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if run == nil {
		notFound(w, "RUN", id)
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (h *RunHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.runs.Delete(id); err != nil {
		notFound(w, "RUN", id)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RunHandlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	events, err := h.runs.ListEvents(id)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": events})
}

func (h *RunHandlers) AddEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	event, err := h.runs.AddEvent(id, req.Type, req.Payload)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

func (h *RunHandlers) Control(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}

	var req engine.ControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if err := h.engine.ControlRun(id, req); err != nil {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", err.Error(), nil)
		return
	}

	// Log control events
	if req.VUs != nil {
		payload, _ := json.Marshal(map[string]int{"vus": *req.VUs})
		h.runs.AddEvent(id, "vu_change", payload)
		if h.ws != nil {
			h.ws.BroadcastRunEvent(id, map[string]interface{}{"type": "vu_change", "vus": *req.VUs})
		}
	}
	if req.RPS != nil {
		payload, _ := json.Marshal(map[string]int{"rps": *req.RPS})
		h.runs.AddEvent(id, "rps_change", payload)
		if h.ws != nil {
			h.ws.BroadcastRunEvent(id, map[string]interface{}{"type": "rps_change", "rps": *req.RPS})
		}
	}
	if req.DurationSeconds != nil {
		payload, _ := json.Marshal(map[string]int{"duration_seconds": *req.DurationSeconds})
		h.runs.AddEvent(id, "duration_change", payload)
		if h.ws != nil {
			h.ws.BroadcastRunEvent(id, map[string]interface{}{"type": "duration_change", "duration_seconds": *req.DurationSeconds})
		}
	}

	// Return current state
	vus, rps, paused, remaining, _ := h.engine.GetRunState(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"vus":              vus,
		"rps":              rps,
		"paused":           paused,
		"remaining_seconds": remaining,
	})
}

func (h *RunHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}

	if err := h.engine.StopRun(id); err != nil {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (h *RunHandlers) Kill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}

	if err := h.engine.KillRun(id); err != nil {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", err.Error(), nil)
		return
	}

	h.runs.UpdateStatus(id, "aborted")
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}
