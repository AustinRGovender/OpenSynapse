package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type RunParameters struct {
	VUsTarget       int `json:"vus_target"`
	RPSTarget       int `json:"rps_target,omitempty"`
	DurationSeconds int `json:"duration_seconds"`
	WorkerCount     int `json:"worker_count"`
}

type RunSummary struct {
	TotalRequests     int64   `json:"total_requests"`
	FailedRequests    int64   `json:"failed_requests"`
	ErrorRate         float64 `json:"error_rate"`
	ThroughputRPS     float64 `json:"throughput_rps"`
	P50MS             float64 `json:"p50_ms"`
	P90MS             float64 `json:"p90_ms"`
	P95MS             float64 `json:"p95_ms"`
	P99MS             float64 `json:"p99_ms"`
	MaxMS             float64 `json:"max_ms"`
	BytesSent         int64   `json:"bytes_sent"`
	BytesReceived     int64   `json:"bytes_received"`
	AssertionFailures int64   `json:"assertion_failures"`
	ThresholdsPassed  bool    `json:"thresholds_passed"`
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
	nowStr := now.Format(time.RFC3339)

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
	var r Run
	var paramsStr, planSnap string
	var envSnap sql.NullString
	var startedAt, endedAt sql.NullString
	var summaryStr sql.NullString
	var createdAt string

	err := s.db.QueryRow(
		`SELECT id, plan_id, plan_version, plan_snapshot, environment_snapshot, parameters, status, started_at, ended_at, summary, created_at
		 FROM runs WHERE id = ?`, id,
	).Scan(&r.ID, &r.PlanID, &r.PlanVersion, &planSnap, &envSnap, &paramsStr, &r.Status, &startedAt, &endedAt, &summaryStr, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}

	r.PlanSnapshot = json.RawMessage(planSnap)
	if envSnap.Valid {
		r.EnvironmentSnapshot = json.RawMessage(envSnap.String)
	}
	json.Unmarshal([]byte(paramsStr), &r.Parameters)
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		r.StartedAt = &t
	}
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		r.EndedAt = &t
	}
	if summaryStr.Valid {
		var summary RunSummary
		json.Unmarshal([]byte(summaryStr.String), &summary)
		r.Summary = &summary
	}

	return &r, nil
}

func (s *RunStore) List(params ListParams) (*ListResult[Run], error) {
	limit := params.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if params.Cursor != "" {
		rows, err = s.db.Query(
			`SELECT id, plan_id, plan_version, parameters, status, started_at, ended_at, summary, created_at
			 FROM runs WHERE created_at < ? ORDER BY created_at DESC LIMIT ?`,
			params.Cursor, limit+1,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, plan_id, plan_version, parameters, status, started_at, ended_at, summary, created_at
			 FROM runs ORDER BY created_at DESC LIMIT ?`,
			limit+1,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var paramsStr string
		var startedAt, endedAt, summaryStr sql.NullString
		var createdAt string

		if err := rows.Scan(&r.ID, &r.PlanID, &r.PlanVersion, &paramsStr, &r.Status, &startedAt, &endedAt, &summaryStr, &createdAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}

		json.Unmarshal([]byte(paramsStr), &r.Parameters)
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			r.StartedAt = &t
		}
		if endedAt.Valid {
			t, _ := time.Parse(time.RFC3339, endedAt.String)
			r.EndedAt = &t
		}
		if summaryStr.Valid {
			var summary RunSummary
			json.Unmarshal([]byte(summaryStr.String), &summary)
			r.Summary = &summary
		}

		runs = append(runs, r)
	}

	result := &ListResult[Run]{Items: runs}
	if len(runs) > limit {
		result.Items = runs[:limit]
		result.NextCursor = runs[limit-1].CreatedAt.Format(time.RFC3339Nano)
	}
	if result.Items == nil {
		result.Items = []Run{}
	}

	return result, nil
}

func (s *RunStore) UpdateStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
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
