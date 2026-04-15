package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SavedRequest struct {
	Name       string            `json:"name"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	BodyType   string            `json:"body_type"`
	AuthType   string            `json:"auth_type"`
	AuthConfig json.RawMessage   `json:"auth_config"`
}

type Collection struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Requests  []SavedRequest `json:"requests"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type CollectionStore struct {
	db *sql.DB
}

func NewCollectionStore(db *sql.DB) *CollectionStore {
	return &CollectionStore{db: db}
}

func (s *CollectionStore) Create(name string, requests []SavedRequest) (*Collection, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if requests == nil {
		requests = []SavedRequest{}
	}
	reqJSON, _ := json.Marshal(requests)
	nowStr := now.Format(time.RFC3339)

	_, err := s.db.Exec(
		`INSERT INTO playground_collections (id, name, requests, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, name, string(reqJSON), nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert collection: %w", err)
	}

	return &Collection{
		ID:        id,
		Name:      name,
		Requests:  requests,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *CollectionStore) Get(id string) (*Collection, error) {
	var c Collection
	var reqStr, createdAt, updatedAt string

	err := s.db.QueryRow(
		`SELECT id, name, requests, created_at, updated_at FROM playground_collections WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &reqStr, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	json.Unmarshal([]byte(reqStr), &c.Requests)
	if c.Requests == nil {
		c.Requests = []SavedRequest{}
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &c, nil
}

func (s *CollectionStore) List() (*ListResult[Collection], error) {
	rows, err := s.db.Query(
		`SELECT id, name, requests, created_at, updated_at FROM playground_collections ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var collections []Collection
	for rows.Next() {
		var c Collection
		var reqStr, createdAt, updatedAt string
		if err := rows.Scan(&c.ID, &c.Name, &reqStr, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		json.Unmarshal([]byte(reqStr), &c.Requests)
		if c.Requests == nil {
			c.Requests = []SavedRequest{}
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		collections = append(collections, c)
	}

	if collections == nil {
		collections = []Collection{}
	}
	return &ListResult[Collection]{Items: collections}, nil
}

func (s *CollectionStore) Update(id, name string, requests []SavedRequest) (*Collection, error) {
	existing, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	if requests == nil {
		requests = []SavedRequest{}
	}
	reqJSON, _ := json.Marshal(requests)
	nowStr := now.Format(time.RFC3339)

	_, err = s.db.Exec(
		`UPDATE playground_collections SET name=?, requests=?, updated_at=? WHERE id=?`,
		name, string(reqJSON), nowStr, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update collection: %w", err)
	}

	return &Collection{
		ID:        id,
		Name:      name,
		Requests:  requests,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
	}, nil
}

func (s *CollectionStore) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM playground_collections WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("collection not found")
	}
	return nil
}
