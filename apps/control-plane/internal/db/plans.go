package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Node struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Enabled    bool            `json:"enabled"`
	Properties json.RawMessage `json:"properties"`
	Children   []Node          `json:"children"`
}

type Plan struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Description          string    `json:"description"`
	Tags                 []string  `json:"tags"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	Version              int       `json:"version"`
	DefaultEnvironmentID *string   `json:"default_environment_id"`
	Root                 Node      `json:"root"`
}

type PlanVersion struct {
	ID          string    `json:"id"`
	PlanID      string    `json:"plan_id"`
	Version     int       `json:"version"`
	Root        Node      `json:"root"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
}

type PlanStore struct {
	db *sql.DB
}

func NewPlanStore(db *sql.DB) *PlanStore {
	return &PlanStore{db: db}
}

type ListParams struct {
	Limit  int
	Cursor string
}

type ListResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func (s *PlanStore) Create(name, description string, tags []string, root Node, defaultEnvID *string) (*Plan, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if tags == nil {
		tags = []string{}
	}

	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}

	rootJSON, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal root: %w", err)
	}

	nowStr := now.Format(time.RFC3339)

	_, err = s.db.Exec(
		`INSERT INTO plans (id, name, description, tags, created_at, updated_at, version, default_environment_id, root)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, name, description, string(tagsJSON), nowStr, nowStr, defaultEnvID, string(rootJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("insert plan: %w", err)
	}

	// Save initial version
	if err := s.saveVersion(id, 1, name, description, tags, root); err != nil {
		return nil, fmt.Errorf("save initial version: %w", err)
	}

	return &Plan{
		ID:                   id,
		Name:                 name,
		Description:          description,
		Tags:                 tags,
		CreatedAt:            now,
		UpdatedAt:            now,
		Version:              1,
		DefaultEnvironmentID: defaultEnvID,
		Root:                 root,
	}, nil
}

func (s *PlanStore) Get(id string) (*Plan, error) {
	var p Plan
	var tagsStr, rootStr string
	var createdAt, updatedAt string
	var envID sql.NullString

	err := s.db.QueryRow(
		`SELECT id, name, description, tags, created_at, updated_at, version, default_environment_id, root
		 FROM plans WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &tagsStr, &createdAt, &updatedAt, &p.Version, &envID, &rootStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsStr), &p.Tags); err != nil {
		p.Tags = []string{}
	}
	if err := json.Unmarshal([]byte(rootStr), &p.Root); err != nil {
		return nil, fmt.Errorf("unmarshal root: %w", err)
	}

	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if envID.Valid {
		p.DefaultEnvironmentID = &envID.String
	}

	return &p, nil
}

func (s *PlanStore) List(params ListParams) (*ListResult[Plan], error) {
	limit := params.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if params.Cursor != "" {
		rows, err = s.db.Query(
			`SELECT id, name, description, tags, created_at, updated_at, version, default_environment_id, root
			 FROM plans WHERE updated_at < ? ORDER BY updated_at DESC LIMIT ?`,
			params.Cursor, limit+1,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, name, description, tags, created_at, updated_at, version, default_environment_id, root
			 FROM plans ORDER BY updated_at DESC LIMIT ?`,
			limit+1,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		var tagsStr, rootStr string
		var createdAt, updatedAt string
		var envID sql.NullString

		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &tagsStr, &createdAt, &updatedAt, &p.Version, &envID, &rootStr); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}

		json.Unmarshal([]byte(tagsStr), &p.Tags)
		if p.Tags == nil {
			p.Tags = []string{}
		}
		json.Unmarshal([]byte(rootStr), &p.Root)
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if envID.Valid {
			p.DefaultEnvironmentID = &envID.String
		}

		plans = append(plans, p)
	}

	result := &ListResult[Plan]{Items: plans}
	if len(plans) > limit {
		result.Items = plans[:limit]
		result.NextCursor = plans[limit-1].UpdatedAt.Format(time.RFC3339Nano)
	}
	if result.Items == nil {
		result.Items = []Plan{}
	}

	return result, nil
}

func (s *PlanStore) Update(id string, name, description string, tags []string, root Node, defaultEnvID *string) (*Plan, error) {
	existing, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	newVersion := existing.Version + 1

	if tags == nil {
		tags = []string{}
	}

	tagsJSON, _ := json.Marshal(tags)
	rootJSON, _ := json.Marshal(root)
	nowStr := now.Format(time.RFC3339)

	_, err = s.db.Exec(
		`UPDATE plans SET name=?, description=?, tags=?, updated_at=?, version=?, default_environment_id=?, root=?
		 WHERE id=?`,
		name, description, string(tagsJSON), nowStr, newVersion, defaultEnvID, string(rootJSON), id,
	)
	if err != nil {
		return nil, fmt.Errorf("update plan: %w", err)
	}

	if err := s.saveVersion(id, newVersion, name, description, tags, root); err != nil {
		return nil, fmt.Errorf("save version: %w", err)
	}

	return &Plan{
		ID:                   id,
		Name:                 name,
		Description:          description,
		Tags:                 tags,
		CreatedAt:            existing.CreatedAt,
		UpdatedAt:            now,
		Version:              newVersion,
		DefaultEnvironmentID: defaultEnvID,
		Root:                 root,
	}, nil
}

func (s *PlanStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM plans WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete plan: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("plan not found")
	}
	return nil
}

func (s *PlanStore) ListVersions(planID string) ([]PlanVersion, error) {
	rows, err := s.db.Query(
		`SELECT id, plan_id, version, root, name, description, tags, created_at
		 FROM plan_versions WHERE plan_id = ? ORDER BY version DESC`, planID,
	)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var versions []PlanVersion
	for rows.Next() {
		var v PlanVersion
		var rootStr, tagsStr, createdAt string
		if err := rows.Scan(&v.ID, &v.PlanID, &v.Version, &rootStr, &v.Name, &v.Description, &tagsStr, &createdAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		json.Unmarshal([]byte(rootStr), &v.Root)
		json.Unmarshal([]byte(tagsStr), &v.Tags)
		if v.Tags == nil {
			v.Tags = []string{}
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		versions = append(versions, v)
	}
	if versions == nil {
		versions = []PlanVersion{}
	}
	return versions, nil
}

func (s *PlanStore) GetVersion(planID string, version int) (*PlanVersion, error) {
	var v PlanVersion
	var rootStr, tagsStr, createdAt string

	err := s.db.QueryRow(
		`SELECT id, plan_id, version, root, name, description, tags, created_at
		 FROM plan_versions WHERE plan_id = ? AND version = ?`, planID, version,
	).Scan(&v.ID, &v.PlanID, &v.Version, &rootStr, &v.Name, &v.Description, &tagsStr, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}

	json.Unmarshal([]byte(rootStr), &v.Root)
	json.Unmarshal([]byte(tagsStr), &v.Tags)
	if v.Tags == nil {
		v.Tags = []string{}
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &v, nil
}

func (s *PlanStore) saveVersion(planID string, version int, name, description string, tags []string, root Node) error {
	id := uuid.New().String()
	tagsJSON, _ := json.Marshal(tags)
	rootJSON, _ := json.Marshal(root)

	_, err := s.db.Exec(
		`INSERT INTO plan_versions (id, plan_id, version, root, name, description, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, planID, version, string(rootJSON), name, description, string(tagsJSON),
	)
	return err
}
