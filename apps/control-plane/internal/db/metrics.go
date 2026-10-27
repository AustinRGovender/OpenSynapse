package db

import (
	"fmt"
)

// MetricPoint is one second of aggregated run metrics. It is both the live
// WebSocket payload (runs.{id}.metrics) and the persisted time series.
// ErrorRate is a percentage (0–100); RPS is requests completed in that second.
type MetricPoint struct {
	TimestampMS   int64   `json:"timestamp_ms"`
	Requests      int64   `json:"requests"`
	Failed        int64   `json:"failed"`
	RPS           float64 `json:"rps"`
	ErrorRate     float64 `json:"error_rate"`
	P50MS         float64 `json:"p50_ms"`
	P90MS         float64 `json:"p90_ms"`
	P95MS         float64 `json:"p95_ms"`
	P99MS         float64 `json:"p99_ms"`
	ActiveVUs     int     `json:"active_vus"`
	BytesSent     int64   `json:"bytes_sent"`
	BytesReceived int64   `json:"bytes_received"`
	Iterations    int64   `json:"iterations"`
	ChecksPassed  int64   `json:"checks_passed"`
	ChecksFailed  int64   `json:"checks_failed"`
}

// RunError is one failed request recorded for the dashboard's error log.
type RunError struct {
	TimestampMS int64  `json:"timestamp_ms"`
	Status      int    `json:"status"`
	Method      string `json:"method"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ErrorCode   string `json:"error_code,omitempty"`
	Message     string `json:"message,omitempty"`
}

// maxStoredErrors caps how many error rows a single run keeps.
const maxStoredErrors = 500

// AddMetrics persists a batch of per-second points for a run.
func (s *RunStore) AddMetrics(runID string, points []MetricPoint) error {
	if len(points) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin metrics tx: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO run_metrics
		(run_id, timestamp_ms, requests, failed, rps, error_rate, p50_ms, p90_ms, p95_ms, p99_ms,
		 active_vus, bytes_sent, bytes_received, iterations, checks_passed, checks_failed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, timestamp_ms) DO NOTHING`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare metrics insert: %w", err)
	}
	defer stmt.Close()
	for _, p := range points {
		if _, err := stmt.Exec(runID, p.TimestampMS, p.Requests, p.Failed, p.RPS, p.ErrorRate,
			p.P50MS, p.P90MS, p.P95MS, p.P99MS, p.ActiveVUs, p.BytesSent, p.BytesReceived,
			p.Iterations, p.ChecksPassed, p.ChecksFailed); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert metric point: %w", err)
		}
	}
	return tx.Commit()
}

// ListMetrics returns a run's per-second series in time order, optionally
// limited to [sinceMS, untilMS] (zero means unbounded).
func (s *RunStore) ListMetrics(runID string, sinceMS, untilMS int64) ([]MetricPoint, error) {
	query := `SELECT timestamp_ms, requests, failed, rps, error_rate, p50_ms, p90_ms, p95_ms, p99_ms,
		active_vus, bytes_sent, bytes_received, iterations, checks_passed, checks_failed
		FROM run_metrics WHERE run_id = ?`
	args := []any{runID}
	if sinceMS > 0 {
		query += " AND timestamp_ms >= ?"
		args = append(args, sinceMS)
	}
	if untilMS > 0 {
		query += " AND timestamp_ms <= ?"
		args = append(args, untilMS)
	}
	query += " ORDER BY timestamp_ms"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list metrics: %w", err)
	}
	defer rows.Close()

	points := []MetricPoint{}
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.TimestampMS, &p.Requests, &p.Failed, &p.RPS, &p.ErrorRate,
			&p.P50MS, &p.P90MS, &p.P95MS, &p.P99MS, &p.ActiveVUs, &p.BytesSent, &p.BytesReceived,
			&p.Iterations, &p.ChecksPassed, &p.ChecksFailed); err != nil {
			return nil, fmt.Errorf("scan metric point: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// AddErrors stores failed-request samples, keeping at most maxStoredErrors per run.
func (s *RunStore) AddErrors(runID string, errs []RunError) error {
	if len(errs) == 0 {
		return nil
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM run_errors WHERE run_id = ?`, runID).Scan(&count); err != nil {
		return fmt.Errorf("count errors: %w", err)
	}
	room := maxStoredErrors - count
	if room <= 0 {
		return nil
	}
	if len(errs) > room {
		errs = errs[:room]
	}
	for _, e := range errs {
		if _, err := s.db.Exec(`INSERT INTO run_errors (run_id, timestamp_ms, status, method, name, url, error_code, message)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, e.TimestampMS, e.Status, e.Method, e.Name, e.URL, e.ErrorCode, e.Message); err != nil {
			return fmt.Errorf("insert error: %w", err)
		}
	}
	return nil
}

// ListErrors returns the most recent errors for a run, newest last.
func (s *RunStore) ListErrors(runID string, limit int) ([]RunError, error) {
	if limit <= 0 || limit > maxStoredErrors {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT timestamp_ms, status, method, name, url, error_code, message
		FROM run_errors WHERE run_id = ? ORDER BY timestamp_ms DESC, id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, fmt.Errorf("list errors: %w", err)
	}
	defer rows.Close()
	out := []RunError{}
	for rows.Next() {
		var e RunError
		if err := rows.Scan(&e.TimestampMS, &e.Status, &e.Method, &e.Name, &e.URL, &e.ErrorCode, &e.Message); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		out = append(out, e)
	}
	// Reverse to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
