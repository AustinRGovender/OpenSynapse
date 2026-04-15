package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Report struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	RunIDs            []string  `json:"run_ids"`
	Metrics           []string  `json:"metrics"`
	Normalisation     string    `json:"normalisation"`
	AIAnalysisCached  *string   `json:"ai_analysis_cached,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ReportStore struct {
	db *sql.DB
}

func NewReportStore(db *sql.DB) *ReportStore {
	return &ReportStore{db: db}
}

func (s *ReportStore) Create(name string, runIDs, metrics []string, normalisation string) (*Report, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if runIDs == nil {
		runIDs = []string{}
	}
	if metrics == nil {
		metrics = []string{}
	}
	if normalisation == "" {
		normalisation = "elapsed_time"
	}

	runIDsJSON, _ := json.Marshal(runIDs)
	metricsJSON, _ := json.Marshal(metrics)
	nowStr := now.Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO reports (id, name, run_ids, metrics, normalisation, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, name, string(runIDsJSON), string(metricsJSON), normalisation, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert report: %w", err)
	}

	return &Report{
		ID:            id,
		Name:          name,
		RunIDs:        runIDs,
		Metrics:       metrics,
		Normalisation: normalisation,
		CreatedAt:     now,
	}, nil
}

func (s *ReportStore) Get(id string) (*Report, error) {
	var r Report
	var runIDsStr, metricsStr, createdAt string
	var aiCache sql.NullString

	err := s.db.QueryRow(
		`SELECT id, name, run_ids, metrics, normalisation, ai_analysis_cached, created_at
		 FROM reports WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &runIDsStr, &metricsStr, &r.Normalisation, &aiCache, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get report: %w", err)
	}

	json.Unmarshal([]byte(runIDsStr), &r.RunIDs)
	json.Unmarshal([]byte(metricsStr), &r.Metrics)
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if aiCache.Valid {
		r.AIAnalysisCached = &aiCache.String
	}

	return &r, nil
}

func (s *ReportStore) List(params ListParams) (*ListResult[Report], error) {
	limit := params.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	rows, err := s.db.Query(
		`SELECT id, name, run_ids, metrics, normalisation, ai_analysis_cached, created_at
		 FROM reports ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()

	var reports []Report
	for rows.Next() {
		var r Report
		var runIDsStr, metricsStr, createdAt string
		var aiCache sql.NullString

		if err := rows.Scan(&r.ID, &r.Name, &runIDsStr, &metricsStr, &r.Normalisation, &aiCache, &createdAt); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}

		json.Unmarshal([]byte(runIDsStr), &r.RunIDs)
		json.Unmarshal([]byte(metricsStr), &r.Metrics)
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if aiCache.Valid {
			r.AIAnalysisCached = &aiCache.String
		}
		reports = append(reports, r)
	}

	if reports == nil {
		reports = []Report{}
	}
	return &ListResult[Report]{Items: reports}, nil
}

func (s *ReportStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM reports WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete report: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("report not found")
	}
	return nil
}
