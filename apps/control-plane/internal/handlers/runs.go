package handlers

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/artifacts"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/auth"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/compiler"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/report"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// RunNotifier receives run lifecycle notifications (used for webhooks).
type RunNotifier interface {
	RunStarted(run *db.Run)
	RunFinished(run *db.Run)
}

// RunHandlers serves the runs API and owns the glue between the engine,
// the run store, and WebSocket subscribers.
type RunHandlers struct {
	runs   *db.RunStore
	plans  *db.PlanStore
	envs   *db.EnvironmentStore
	engine *engine.Engine
	ws     *wsserver.Server

	// ControlBaseURL is how k6 (running next to the control plane) reaches
	// the control-state endpoint, e.g. http://127.0.0.1:8090.
	ControlBaseURL string
	// Artifacts, if set, receives each run's raw-sample archive.
	Artifacts artifacts.Store
	// Notifier, if set, is told about run starts and finishes.
	Notifier RunNotifier
}

func NewRunHandlers(runs *db.RunStore, plans *db.PlanStore, envs *db.EnvironmentStore, eng *engine.Engine, ws *wsserver.Server) *RunHandlers {
	return &RunHandlers{runs: runs, plans: plans, envs: envs, engine: eng, ws: ws}
}

type startRunRequest struct {
	PlanID        string            `json:"plan_id"`
	EnvironmentID *string           `json:"environment_id"`
	Parameters    *db.RunParameters `json:"parameters"`
}

// Limits on per-run overrides; beyond these a single machine cannot cope anyway.
const (
	maxWorkers     = 64
	maxRunVUs      = 100000
	maxRunDuration = 7 * 24 * time.Hour
)

func (h *RunHandlers) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	result, err := h.runs.List(db.RunFilter{
		ListParams: db.ListParams{Limit: limit, Cursor: q.Get("cursor")},
		PlanID:     q.Get("plan_id"),
		Status:     q.Get("status"),
		Since:      q.Get("since"),
		Until:      q.Get("until"),
	})
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validateParameters(p *db.RunParameters) error {
	if p.VUsTarget < 0 || p.VUsTarget > maxRunVUs {
		return fmt.Errorf("vus_target must be between 0 and %d", maxRunVUs)
	}
	if p.RPSTarget < 0 {
		return fmt.Errorf("rps_target must be zero or more")
	}
	if p.WorkerCount < 1 || p.WorkerCount > maxWorkers {
		return fmt.Errorf("worker_count must be between 1 and %d", maxWorkers)
	}
	if p.DurationSeconds < 0 || time.Duration(p.DurationSeconds)*time.Second > maxRunDuration {
		return fmt.Errorf("duration_seconds must be between 0 and %d", int(maxRunDuration.Seconds()))
	}
	var total time.Duration
	for i, s := range p.Stages {
		d, err := compiler.ParseDuration(s.Duration)
		if err != nil {
			return fmt.Errorf("stage %d: %v", i+1, err)
		}
		if s.Target < 0 || s.Target > maxRunVUs {
			return fmt.Errorf("stage %d: target must be between 0 and %d", i+1, maxRunVUs)
		}
		total += d
	}
	if total > maxRunDuration {
		return fmt.Errorf("stages add up to more than %s", maxRunDuration)
	}
	return nil
}

func randomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *RunHandlers) Start(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE",
			"k6 engine is not available. Install k6 (or use the Docker image, which bundles it) to run tests.", nil)
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

	plan, err := h.plans.Get(req.PlanID)
	if err != nil {
		internalError(w, err)
		return
	}
	if plan == nil {
		notFound(w, "PLAN", req.PlanID)
		return
	}

	params := db.RunParameters{WorkerCount: 1}
	if req.Parameters != nil {
		params = *req.Parameters
	}
	if params.WorkerCount <= 0 {
		params.WorkerCount = 1
	}
	if err := validateParameters(&params); err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if exec := h.engine.Execution(); params.WorkerCount > exec.MaxWorkers {
		msg := fmt.Sprintf("worker_count can be at most %d", exec.MaxWorkers)
		if exec.Mode == "local" {
			msg = "worker_count above 1 needs cluster mode; a local install runs one k6 process"
		}
		badRequest(w, "VALIDATION_ERROR", msg, nil)
		return
	}

	// Resolve the environment: explicit choice, else the plan's default.
	envID := ""
	if req.EnvironmentID != nil {
		envID = *req.EnvironmentID
	} else if plan.DefaultEnvironmentID != nil {
		envID = *plan.DefaultEnvironmentID
	}
	var envVars map[string]string
	var envSnapshot json.RawMessage
	if envID != "" && h.envs != nil {
		env, err := h.envs.Get(envID)
		if err != nil {
			internalError(w, err)
			return
		}
		if env == nil {
			notFound(w, "ENVIRONMENT", envID)
			return
		}
		envVars = map[string]string{}
		for k, v := range env.Variables {
			envVars[k] = v.Value
		}
		envSnapshot, _ = json.Marshal(env.Redacted())
	}

	caps := h.engine.Capabilities()
	compiled, err := compiler.CompileWithOptions(plan, compiler.Options{Managed: true, KV: caps.KV, Overrides: &params, Instances: params.WorkerCount})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "COMPILATION_FAILED", err.Error(), nil)
		return
	}

	// Record what will actually run so history reflects reality.
	if compiled.Profile != nil {
		params.VUsTarget = compiled.Profile.Peak()
		params.DurationSeconds = int(compiled.PlannedDuration.Seconds())
		if len(params.Stages) == 0 && len(compiled.Profile.Stages) > 1 {
			for _, s := range compiled.Profile.Stages {
				params.Stages = append(params.Stages, db.RunStage{Duration: s.Duration.String(), Target: s.Target})
			}
		}
	}

	planJSON, _ := json.Marshal(plan)
	run, err := h.runs.Create(plan.ID, plan.Version, planJSON, envSnapshot, params)
	if err != nil {
		internalError(w, err)
		return
	}
	if p := auth.FromContext(r.Context()); p != nil && p.UserID != "" {
		h.runs.SetCreatedBy(run.ID, p.UserID)
	}

	spec := engine.RunSpec{
		RunID:           run.ID,
		Script:          compiled.Script,
		Env:             envVars,
		Instances:       params.WorkerCount,
		MaxVUs:          compiled.MaxVUs,
		PlannedDuration: compiled.PlannedDuration,
		MaxDuration:     compiled.MaxDuration,
		RPSTarget:       params.RPSTarget,
	}
	if compiled.Profile != nil {
		sched := engine.Schedule{StartVUs: compiled.Profile.StartVUs}
		for _, s := range compiled.Profile.Stages {
			sched.Stages = append(sched.Stages, engine.Stage{Duration: s.Duration, Target: s.Target})
		}
		spec.Schedule = &sched
	}
	spec.ControlToken = randomToken()
	spec.IngestURL = h.ControlBaseURL + "/api/v1/runs/" + run.ID + "/ingest"
	if compiled.Control {
		spec.ControlURL = h.ControlBaseURL + "/api/v1/runs/" + run.ID + "/control-state"
	}
	if h.Artifacts != nil {
		key := artifacts.RunSamplesKey(run.ID)
		spec.RawSamples = func() (io.WriteCloser, error) { return h.Artifacts.Create(context.Background(), key) }
	}

	runID := run.ID
	err = h.engine.StartRun(spec, engine.Callbacks{
		OnMetrics: func(points []db.MetricPoint, state engine.LiveState) {
			if points == nil {
				points = []db.MetricPoint{} // always an array on the wire
			}
			if err := h.runs.AddMetrics(runID, points); err != nil {
				log.Printf("run %s: store metrics: %v", runID, err)
			}
			if h.ws != nil {
				h.ws.Broadcast("runs."+runID+".metrics", map[string]any{"points": points, "state": state})
			}
		},
		OnErrors: func(errs []db.RunError) {
			if err := h.runs.AddErrors(runID, errs); err != nil {
				log.Printf("run %s: store errors: %v", runID, err)
			}
			if h.ws != nil {
				h.ws.Broadcast("runs."+runID+".errors", map[string]any{"errors": errs})
			}
		},
		OnComplete: func(res engine.Result) {
			h.finishRun(runID, res)
		},
	})
	if err != nil {
		h.runs.UpdateStatus(runID, "failed")
		h.addEvent(runID, "error", map[string]any{"message": "Failed to start k6: " + err.Error()})
		writeError(w, http.StatusInternalServerError, "ENGINE_ERROR", "Failed to start k6: "+err.Error(), nil)
		return
	}

	h.runs.UpdateStatus(runID, "running")
	startPayload := map[string]any{"vus": params.VUsTarget, "duration_seconds": params.DurationSeconds}
	if params.RPSTarget > 0 {
		startPayload["rps"] = params.RPSTarget
	}
	if envID != "" {
		startPayload["environment_id"] = envID
	}
	h.addEvent(runID, "start", startPayload)

	run, _ = h.runs.Get(runID)
	h.broadcastStatus(run)
	if h.Notifier != nil {
		h.Notifier.RunStarted(run)
	}
	writeJSON(w, http.StatusCreated, run)
}

// finishRun persists a run's outcome and tells subscribers.
func (h *RunHandlers) finishRun(runID string, res engine.Result) {
	if res.Summary != nil {
		if err := h.runs.UpdateSummary(runID, res.Summary); err != nil {
			log.Printf("run %s: store summary: %v", runID, err)
		}
	}
	// Events first, then the status: whoever sees the run finished also
	// sees why it finished.
	if res.Status == "failed" && res.Message != "" {
		h.addEvent(runID, "error", map[string]any{"message": res.Message})
	}
	payload := map[string]any{"status": res.Status, "exit_code": res.ExitCode}
	if res.Message != "" {
		payload["message"] = res.Message
	}
	h.addEvent(runID, "stop", payload)
	h.runs.UpdateStatus(runID, res.Status)

	run, err := h.runs.Get(runID)
	if err != nil || run == nil {
		return
	}
	h.broadcastStatus(run)
	if h.Notifier != nil {
		h.Notifier.RunFinished(run)
	}
}

func (h *RunHandlers) broadcastStatus(run *db.Run) {
	if h.ws == nil || run == nil {
		return
	}
	h.ws.Broadcast("runs."+run.ID+".status", map[string]any{
		"status":     run.Status,
		"started_at": run.StartedAt,
		"ended_at":   run.EndedAt,
		"summary":    run.Summary,
	})
}

// addEvent stores a run event and broadcasts it on runs.{id}.events.
func (h *RunHandlers) addEvent(runID, typ string, payload map[string]any) {
	raw, _ := json.Marshal(payload)
	ev, err := h.runs.AddEvent(runID, typ, raw)
	if err != nil {
		log.Printf("run %s: store %s event: %v", runID, typ, err)
		return
	}
	if h.ws != nil {
		h.ws.Broadcast("runs."+runID+".events", ev)
	}
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
	if h.engine != nil && h.engine.IsRunning(id) {
		writeError(w, http.StatusConflict, "RUN_ACTIVE", "Stop the run before deleting it", nil)
		return
	}
	if err := h.runs.Delete(id); err != nil {
		notFound(w, "RUN", id)
		return
	}
	if h.Artifacts != nil {
		if err := h.Artifacts.DeletePrefix(r.Context(), artifacts.RunPrefix(id)); err != nil {
			log.Printf("run %s: delete artifacts: %v", id, err)
		}
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

// AddEvent lets users attach a note to a run's timeline.
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
	if req.Type == "" {
		req.Type = "user_note"
	}
	if req.Type != "user_note" {
		badRequest(w, "VALIDATION_ERROR", "only user_note events can be added", nil)
		return
	}
	run, err := h.runs.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if run == nil {
		notFound(w, "RUN", id)
		return
	}

	event, err := h.runs.AddEvent(id, req.Type, req.Payload)
	if err != nil {
		internalError(w, err)
		return
	}
	if h.ws != nil {
		h.ws.Broadcast("runs."+id+".events", event)
	}
	writeJSON(w, http.StatusCreated, event)
}

// Samples returns the run's per-second metric series.
func (h *RunHandlers) Samples(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	until, _ := strconv.ParseInt(r.URL.Query().Get("until"), 10, 64)
	run, err := h.runs.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if run == nil {
		notFound(w, "RUN", id)
		return
	}
	points, err := h.runs.ListMetrics(id, since, until)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": points})
}

// Errors returns the most recent failed requests.
func (h *RunHandlers) Errors(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	errs, err := h.runs.ListErrors(id, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": errs})
}

// RunReport is the full report document for a run.
type RunReport = report.Document

// BuildReport assembles the report document for a run (nil if not found).
func (h *RunHandlers) BuildReport(id string) (*RunReport, error) {
	run, err := h.runs.Get(id)
	if err != nil || run == nil {
		return nil, err
	}
	series, err := h.runs.ListMetrics(id, 0, 0)
	if err != nil {
		return nil, err
	}
	events, err := h.runs.ListEvents(id)
	if err != nil {
		return nil, err
	}
	errs, err := h.runs.ListErrors(id, 500)
	if err != nil {
		return nil, err
	}
	return &RunReport{Run: run, Series: series, Events: events, Errors: errs}, nil
}

func (h *RunHandlers) Report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rep, err := h.BuildReport(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if rep == nil {
		notFound(w, "RUN", id)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// Live returns the live-control state of a running test.
func (h *RunHandlers) Live(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}
	st, err := h.engine.State(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type controlRequest struct {
	VUs             *int  `json:"vus,omitempty"`
	RPS             *int  `json:"rps,omitempty"`
	DurationSeconds *int  `json:"duration_seconds,omitempty"`
	Paused          *bool `json:"paused,omitempty"`
	ResumeSchedule  bool  `json:"resume_schedule,omitempty"`
}

// Control applies live changes. Each requested change is applied in turn;
// the first one k6 or the engine rejects stops processing and is reported.
func (h *RunHandlers) Control(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}

	var req controlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if !h.engine.IsRunning(id) {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
		return
	}

	fail := func(err error) {
		switch {
		case errors.Is(err, engine.ErrNotActive):
			writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
		case errors.Is(err, engine.ErrUnsupported):
			writeError(w, http.StatusUnprocessableEntity, "CONTROL_UNSUPPORTED", err.Error(), nil)
		default:
			writeError(w, http.StatusUnprocessableEntity, "CONTROL_REJECTED", err.Error(), nil)
		}
	}

	if req.Paused != nil {
		if err := h.engine.SetPaused(id, *req.Paused); err != nil {
			fail(err)
			return
		}
		if *req.Paused {
			h.addEvent(id, "pause", map[string]any{})
		} else {
			h.addEvent(id, "resume", map[string]any{})
		}
	}
	if req.VUs != nil {
		if err := h.engine.SetVUs(id, *req.VUs); err != nil {
			fail(err)
			return
		}
		h.addEvent(id, "vu_change", map[string]any{"vus": *req.VUs})
	}
	if req.ResumeSchedule {
		if err := h.engine.ResumeSchedule(id); err != nil {
			fail(err)
			return
		}
		h.addEvent(id, "vu_change", map[string]any{"schedule": "resumed"})
	}
	if req.RPS != nil {
		if err := h.engine.SetRPS(id, *req.RPS); err != nil {
			fail(err)
			return
		}
		h.addEvent(id, "rps_change", map[string]any{"rps": *req.RPS})
	}
	if req.DurationSeconds != nil {
		if err := h.engine.SetDuration(id, *req.DurationSeconds); err != nil {
			fail(err)
			return
		}
		h.addEvent(id, "duration_change", map[string]any{"duration_seconds": *req.DurationSeconds})
	}

	st, err := h.engine.State(id)
	if err != nil {
		fail(err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ControlState is polled by the run's internal k6 control scenario. It is
// authenticated with the per-run token rather than user credentials.
func (h *RunHandlers) ControlState(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}
	st, err := h.engine.ControlStateFor(r.PathValue("id"), r.Header.Get("X-OpenSynapse-Run-Token"))
	if err != nil {
		if errors.Is(err, engine.ErrNotActive) {
			writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
			return
		}
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Invalid run token", nil)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// maxIngestBody bounds one worker batch (decompressed).
const maxIngestBody = 16 << 20

// Ingest receives aggregated metrics from a cluster run's workers. Like
// control-state it is authenticated by the run's token, not a user's.
func (h *RunHandlers) Ingest(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}
	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			badRequest(w, "INVALID_BODY", "Body is not valid gzip", nil)
			return
		}
		defer zr.Close()
		body = zr
	}
	var batch engine.IngestBatch
	if err := json.NewDecoder(io.LimitReader(body, maxIngestBody)).Decode(&batch); err != nil {
		badRequest(w, "INVALID_JSON", "Body is not a valid metrics batch", nil)
		return
	}
	if err := h.engine.Ingest(r.PathValue("id"), r.Header.Get("X-OpenSynapse-Run-Token"), &batch); err != nil {
		if errors.Is(err, engine.ErrNotActive) {
			writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
			return
		}
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RunHandlers) Stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}

	if err := h.engine.StopRun(id); err != nil {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
		return
	}
	h.addEvent(id, "stop_requested", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

func (h *RunHandlers) Kill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available", nil)
		return
	}

	if err := h.engine.KillRun(id); err != nil {
		writeError(w, http.StatusConflict, "RUN_NOT_ACTIVE", "The run is not executing", nil)
		return
	}
	h.addEvent(id, "kill", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}
