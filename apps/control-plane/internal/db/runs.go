package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RunStage is one step of a ramping load profile.
type RunStage struct {
	Duration string `json:"duration"` // k6 duration string, e.g. "2m" or "30s"
	Target   int    `json:"target"`
}

// RunParameters are the per-run overrides chosen in the Run dialog. Zero
// values mean "use the plan's scenario settings".
type RunParameters struct {
	VUsTarget       int        `json:"vus_target"`
	RPSTarget       int        `json:"rps_target,omitempty"`
	DurationSeconds int        `json:"duration_seconds"`
	Stages          []RunStage `json:"stages,omitempty"`
	WorkerCount     int        `json:"worker_count"`
}

// RunSummary is computed from the run's raw samples when the run ends.
// ErrorRate is a percentage (0–100).
type RunSummary struct {
	TotalRequests     int64          `json:"total_requests"`
	FailedRequests    int64          `json:"failed_requests"`
	ErrorRate         float64        `json:"error_rate"`
	ThroughputRPS     float64        `json:"throughput_rps"`
	AvgMS             float64        `json:"avg_ms"`
	MinMS             float64        `json:"min_ms"`
	P50MS             float64        `json:"p50_ms"`
	P90MS             float64        `json:"p90_ms"`
	P95MS             float64        `json:"p95_ms"`
	P99MS             float64        `json:"p99_ms"`
	MaxMS             float64        `json:"max_ms"`
	BytesSent         int64          `json:"bytes_sent"`
	BytesReceived     int64          `json:"bytes_received"`
	Iterations        int64          `json:"iterations"`
	AssertionPasses   int64          `json:"assertion_passes"`
	AssertionFailures int64          `json:"assertion_failures"`
	ThresholdsPassed  bool           `json:"thresholds_passed"`
	PeakVUs           int            `json:"peak_vus"`
	DurationSeconds   float64        `json:"duration_seconds"`
	Endpoints         []EndpointStat `json:"endpoints,omitempty"`
	Checks            []CheckStat    `json:"checks,omitempty"`
}

// EndpointStat is the per-endpoint breakdown in a run summary.
type EndpointStat struct {
	Method    string  `json:"method"`
	Name      string  `json:"name"`
	Requests  int64   `json:"requests"`
	Failed    int64   `json:"failed"`
	ErrorRate float64 `json:"error_rate"`
	RPS       float64 `json:"rps"`
	AvgMS     float64 `json:"avg_ms"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
	MaxMS     float64 `json:"max_ms"`
}

// CheckStat is the pass/fail tally for one assertion.
type CheckStat struct {
	Name   string `json:"name"`
	Group  string `json:"group"`
	Passes int64  `json:"passes"`
	Fails  int64  `json:"fails"`
}

type Run struct {
	ID                  string         `json:"id"`
	PlanID              string         `json:"plan_id"`
	PlanVersion         int            `json:"plan_version"`
	PlanSnapshot        json.RawMessage `json:"plan_snapshot"`
	EnvironmentSnapshot json.RawMessage `json:"environment_snapshot,omitempty"`
	Parameters          RunParameters  `json:"parameters"`
	Status              string         `json:"status"`
	StartedAt           *time.Time     `json:"started_at"`
	EndedAt             *time.Time     `json:"ended_at"`
	Summary             *RunSummary    `json:"summary"`
	CreatedAt           time.Time      `json:"created_at"`
	// PlanName is the plan's current name (or its name at run time if the
	// plan has since been deleted).
	PlanName string `json:"plan_name"`
}

// RunFilter narrows a run listing.
type RunFilter struct {
	ListParams
	PlanID string
	Status string // queued | running | completed | failed | aborted
	Since  string // RFC 3339; runs created at or after
	Until  string // RFC 3339; runs created at or before
}

// timestampFormat is fixed-width so TEXT timestamps sort chronologically.
const timestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

const runColumns = `r.id, r.plan_id, r.plan_version, r.environment_snapshot, r.parameters, r.status,
	r.started_at, r.ended_at, r.summary, r.created_at,
	COALESCE(p.name, json_extract(r.plan_snapshot, '$.name'), '')`

func scanRun(scan func(dest ...any) error) (*Run, error) {
	var r Run
	var planID sql.NullString
	var paramsStr string
	var envSnap, startedAt, endedAt, summaryStr sql.NullString
	var createdAt string
	if err := scan(&r.ID, &planID, &r.PlanVersion, &envSnap, &paramsStr, &r.Status,
		&startedAt, &endedAt, &summaryStr, &createdAt, &r.PlanName); err != nil {
		return nil, err
	}
	r.PlanID = planID.String
	if envSnap.Valid {
		r.EnvironmentSnapshot = json.RawMessage(envSnap.String)
	}
	json.Unmarshal([]byte(paramsStr), &r.Parameters)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		r.StartedAt = &t
	}
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, endedAt.String)
		r.EndedAt = &t
	}
	if summaryStr.Valid {
		var summary RunSummary
		json.Unmarshal([]byte(summaryStr.String), &summary)
		r.Summary = &summary
	}
	return &r, nil
}

type RunEvent struct {
	ID          string          `json:"id"`
	RunID       string          `json:"run_id"`
	TimestampMS int64           `json:"timestamp_ms"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
}

type RunStore struct {
	db *sql.DB
}

func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

func (s *RunStore) Create(planID string, planVersion int, planSnapshot json.RawMessage, envSnapshot json.RawMessage, params RunParameters) (*Run, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	paramsJSON, _ := json.Marshal(params)
	nowStr := now.Format(timestampFormat)

	var envSnap *string
	if envSnapshot != nil {
		es := string(envSnapshot)
		envSnap = &es
	}

	_, err := s.db.Exec(
		`INSERT INTO runs (id, plan_id, plan_version, plan_snapshot, environment_snapshot, parameters, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'queued', ?)`,
		id, planID, planVersion, string(planSnapshot), envSnap, string(paramsJSON), nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert run: %w", err)
	}

	return &Run{
		ID:                  id,
		PlanID:              planID,
		PlanVersion:         planVersion,
		PlanSnapshot:        planSnapshot,
		EnvironmentSnapshot: envSnapshot,
		Parameters:          params,
		Status:              "queued",
		CreatedAt:           now,
	}, nil
}

func (s *RunStore) Get(id string) (*Run, error) {
	row := s.db.QueryRow(`SELECT `+runColumns+`, r.plan_snapshot
		FROM runs r LEFT JOIN plans p ON p.id = r.plan_id WHERE r.id = ?`, id)
	var planSnap string
	run, err := scanRun(func(dest ...any) error { return row.Scan(append(dest, &planSnap)...) })
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	run.PlanSnapshot = json.RawMessage(planSnap)
	return run, nil
}

// List returns runs newest first. Summaries are included; plan snapshots are not.
func (s *RunStore) List(f RunFilter) (*ListResult[Run], error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `SELECT ` + runColumns + ` FROM runs r LEFT JOIN plans p ON p.id = r.plan_id WHERE 1=1`
	var args []any
	if f.Cursor != "" {
		query += " AND r.created_at < ?"
		args = append(args, f.Cursor)
	}
	if f.PlanID != "" {
		query += " AND r.plan_id = ?"
		args = append(args, f.PlanID)
	}
	if f.Status != "" && f.Status != "all" {
		query += " AND r.status = ?"
		args = append(args, f.Status)
	}
	if f.Since != "" {
		if t, err := time.Parse(time.RFC3339, f.Since); err == nil {
			query += " AND r.created_at >= ?"
			args = append(args, t.UTC().Format(timestampFormat))
		}
	}
	if f.Until != "" {
		if t, err := time.Parse(time.RFC3339, f.Until); err == nil {
			query += " AND r.created_at <= ?"
			args = append(args, t.UTC().Format(timestampFormat))
		}
	}
	query += " ORDER BY r.created_at DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	runs := []Run{}
	var cursors []string
	for rows.Next() {
		r, err := scanRun(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		runs = append(runs, *r)
		cursors = append(cursors, r.CreatedAt.UTC().Format(timestampFormat))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := &ListResult[Run]{Items: runs}
	if len(runs) > limit {
		result.Items = runs[:limit]
		result.NextCursor = cursors[limit-1]
	}
	return result, nil
}

// MarkInterrupted fails runs that were in flight when the control plane
// stopped; their k6 processes did not survive the restart.
func (s *RunStore) MarkInterrupted() (int64, error) {
	now := time.Now().UTC().Format(timestampFormat)
	rows, err := s.db.Query(`SELECT id FROM runs WHERE status IN ('queued', 'running')`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		s.db.Exec(`UPDATE runs SET status = 'failed', ended_at = ? WHERE id = ?`, now, id)
		s.AddEvent(id, "error", json.RawMessage(`{"message":"The control plane restarted while this run was in progress."}`))
	}
	return int64(len(ids)), nil
}

func (s *RunStore) UpdateStatus(id, status string) error {
	now := time.Now().UTC().Format(timestampFormat)
	switch status {
	case "running":
		_, err := s.db.Exec("UPDATE runs SET status=?, started_at=? WHERE id=?", status, now, id)
		return err
	case "completed", "failed", "aborted":
		_, err := s.db.Exec("UPDATE runs SET status=?, ended_at=? WHERE id=?", status, now, id)
		return err
	default:
		_, err := s.db.Exec("UPDATE runs SET status=? WHERE id=?", status, id)
		return err
	}
}

func (s *RunStore) UpdateSummary(id string, summary *RunSummary) error {
	summaryJSON, _ := json.Marshal(summary)
	_, err := s.db.Exec("UPDATE runs SET summary=? WHERE id=?", string(summaryJSON), id)
	return err
}

func (s *RunStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM runs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete run: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("run not found")
	}
	return nil
}

// Events

func (s *RunStore) AddEvent(runID, eventType string, payload json.RawMessage) (*RunEvent, error) {
	id := uuid.New().String()
	ts := time.Now().UnixMilli()

	if payload == nil {
		payload = json.RawMessage("{}")
	}

	_, err := s.db.Exec(
		`INSERT INTO run_events (id, run_id, timestamp_ms, type, payload) VALUES (?, ?, ?, ?, ?)`,
		id, runID, ts, eventType, string(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("add event: %w", err)
	}

	return &RunEvent{
		ID:          id,
		RunID:       runID,
		TimestampMS: ts,
		Type:        eventType,
		Payload:     payload,
	}, nil
}

func (s *RunStore) ListEvents(runID string) ([]RunEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, run_id, timestamp_ms, type, payload FROM run_events WHERE run_id = ? ORDER BY timestamp_ms`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []RunEvent
	for rows.Next() {
		var e RunEvent
		var payload string
		if err := rows.Scan(&e.ID, &e.RunID, &e.TimestampMS, &e.Type, &payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.Payload = json.RawMessage(payload)
		events = append(events, e)
	}
	if events == nil {
		events = []RunEvent{}
	}
	return events, nil
}
