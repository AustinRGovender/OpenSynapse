package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Variable struct {
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

type Environment struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Variables map[string]Variable `json:"variables"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type EnvironmentStore struct {
	db *sql.DB
}

func NewEnvironmentStore(db *sql.DB) *EnvironmentStore {
	return &EnvironmentStore{db: db}
}

func (s *EnvironmentStore) Create(name string, variables map[string]Variable) (*Environment, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if variables == nil {
		variables = map[string]Variable{}
	}

	varsJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	nowStr := now.Format(time.RFC3339)

	_, err = s.db.Exec(
		`INSERT INTO environments (id, name, variables, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, name, string(varsJSON), nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert environment: %w", err)
	}

	return &Environment{
		ID:        id,
		Name:      name,
		Variables: variables,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *EnvironmentStore) Get(id string) (*Environment, error) {
	var e Environment
	var varsStr, createdAt, updatedAt string

	err := s.db.QueryRow(
		`SELECT id, name, variables, created_at, updated_at FROM environments WHERE id = ?`, id,
	).Scan(&e.ID, &e.Name, &varsStr, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get environment: %w", err)
	}

	if err := json.Unmarshal([]byte(varsStr), &e.Variables); err != nil {
		e.Variables = map[string]Variable{}
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &e, nil
}

func (s *EnvironmentStore) List(params ListParams) (*ListResult[Environment], error) {
	limit := params.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if params.Cursor != "" {
		rows, err = s.db.Query(
			`SELECT id, name, variables, created_at, updated_at
			 FROM environments WHERE updated_at < ? ORDER BY updated_at DESC LIMIT ?`,
			params.Cursor, limit+1,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, name, variables, created_at, updated_at
			 FROM environments ORDER BY updated_at DESC LIMIT ?`,
			limit+1,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	var envs []Environment
	for rows.Next() {
		var e Environment
		var varsStr, createdAt, updatedAt string

		if err := rows.Scan(&e.ID, &e.Name, &varsStr, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan environment: %w", err)
		}

		json.Unmarshal([]byte(varsStr), &e.Variables)
		if e.Variables == nil {
			e.Variables = map[string]Variable{}
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		envs = append(envs, e)
	}

	result := &ListResult[Environment]{Items: envs}
	if len(envs) > limit {
		result.Items = envs[:limit]
		result.NextCursor = envs[limit-1].UpdatedAt.Format(time.RFC3339Nano)
	}
	if result.Items == nil {
		result.Items = []Environment{}
	}

	return result, nil
}

func (s *EnvironmentStore) Update(id, name string, variables map[string]Variable) (*Environment, error) {
	existing, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	if variables == nil {
		variables = map[string]Variable{}
	}

	varsJSON, _ := json.Marshal(variables)
	nowStr := now.Format(time.RFC3339)

	_, err = s.db.Exec(
		`UPDATE environments SET name=?, variables=?, updated_at=? WHERE id=?`,
		name, string(varsJSON), nowStr, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update environment: %w", err)
	}

	return &Environment{
		ID:        id,
		Name:      name,
		Variables: variables,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
	}, nil
}

func (s *EnvironmentStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM environments WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("environment not found")
	}
	return nil
}
