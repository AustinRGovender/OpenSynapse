package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CrawlAuthConfig struct {
	Type     string `json:"type"`      // none, form, bearer, basic
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

type CrawlProgress struct {
	PagesDiscovered  int `json:"pages_discovered"`
	RequestsCaptured int `json:"requests_captured"`
}

type CrawlGraphNode struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type CrawlGraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type CrawlGraph struct {
	Nodes []CrawlGraphNode `json:"nodes"`
	Edges []CrawlGraphEdge `json:"edges"`
}

type CapturedRequest struct {
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body,omitempty"`
	StatusCode int               `json:"status_code"`
}

type Crawl struct {
	ID              string          `json:"id"`
	EntryURL        string          `json:"entry_url"`
	AuthConfig      CrawlAuthConfig `json:"auth_config"`
	Depth           int             `json:"depth"`
	SameOrigin      bool            `json:"same_origin"`
	Blocklist       []string        `json:"blocklist"`
	RequestLimit    int             `json:"request_limit"`
	Status          string          `json:"status"`
	Progress        CrawlProgress   `json:"progress"`
	Graph           CrawlGraph      `json:"graph"`
	Requests        []CapturedRequest `json:"requests"`
	OpenAPIURL      string          `json:"openapi_url,omitempty"`
	OpenAPISpec     string          `json:"openapi_spec,omitempty"`
	GeneratedPlanID *string         `json:"generated_plan_id,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type CrawlStore struct {
	db *sql.DB
}

func NewCrawlStore(db *sql.DB) *CrawlStore {
	return &CrawlStore{db: db}
}

func (s *CrawlStore) Create(entryURL string, auth CrawlAuthConfig, depth int, sameOrigin bool, blocklist []string, limit int, openapiURL string) (*Crawl, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if blocklist == nil {
		blocklist = []string{}
	}

	authJSON, _ := json.Marshal(auth)
	blocklistJSON, _ := json.Marshal(blocklist)
	progressJSON, _ := json.Marshal(CrawlProgress{})
	graphJSON, _ := json.Marshal(CrawlGraph{Nodes: []CrawlGraphNode{}, Edges: []CrawlGraphEdge{}})
	nowStr := now.Format(time.RFC3339)

	sameOriginInt := 0
	if sameOrigin {
		sameOriginInt = 1
	}

	var openapiURLPtr *string
	if openapiURL != "" {
		openapiURLPtr = &openapiURL
	}

	_, err := s.db.Exec(
		`INSERT INTO crawls (id, entry_url, auth_config, depth, same_origin, blocklist, request_limit, status, progress, graph, requests, openapi_url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, '[]', ?, ?, ?)`,
		id, entryURL, string(authJSON), depth, sameOriginInt, string(blocklistJSON), limit,
		string(progressJSON), string(graphJSON), openapiURLPtr, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert crawl: %w", err)
	}

	return &Crawl{
		ID:           id,
		EntryURL:     entryURL,
		AuthConfig:   auth,
		Depth:        depth,
		SameOrigin:   sameOrigin,
		Blocklist:    blocklist,
		RequestLimit: limit,
		Status:       "pending",
		Progress:     CrawlProgress{},
		Graph:        CrawlGraph{Nodes: []CrawlGraphNode{}, Edges: []CrawlGraphEdge{}},
		Requests:     []CapturedRequest{},
		OpenAPIURL:   openapiURL,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *CrawlStore) Get(id string) (*Crawl, error) {
	var c Crawl
	var authStr, blocklistStr, progressStr, graphStr, requestsStr string
	var openapiURL, openapiSpec, genPlanID, errMsg sql.NullString
	var sameOriginInt int
	var createdAt, updatedAt string

	err := s.db.QueryRow(
		`SELECT id, entry_url, auth_config, depth, same_origin, blocklist, request_limit, status,
		        progress, graph, requests, openapi_url, openapi_spec, generated_plan_id, error_message,
		        created_at, updated_at
		 FROM crawls WHERE id = ?`, id,
	).Scan(&c.ID, &c.EntryURL, &authStr, &c.Depth, &sameOriginInt, &blocklistStr, &c.RequestLimit,
		&c.Status, &progressStr, &graphStr, &requestsStr, &openapiURL, &openapiSpec, &genPlanID,
		&errMsg, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get crawl: %w", err)
	}

	c.SameOrigin = sameOriginInt == 1
	json.Unmarshal([]byte(authStr), &c.AuthConfig)
	json.Unmarshal([]byte(blocklistStr), &c.Blocklist)
	json.Unmarshal([]byte(progressStr), &c.Progress)
	json.Unmarshal([]byte(graphStr), &c.Graph)
	json.Unmarshal([]byte(requestsStr), &c.Requests)
	if c.Blocklist == nil { c.Blocklist = []string{} }
	if c.Graph.Nodes == nil { c.Graph.Nodes = []CrawlGraphNode{} }
	if c.Graph.Edges == nil { c.Graph.Edges = []CrawlGraphEdge{} }
	if c.Requests == nil { c.Requests = []CapturedRequest{} }
	if openapiURL.Valid { c.OpenAPIURL = openapiURL.String }
	if openapiSpec.Valid { c.OpenAPISpec = openapiSpec.String }
	if genPlanID.Valid { c.GeneratedPlanID = &genPlanID.String }
	if errMsg.Valid { c.ErrorMessage = errMsg.String }
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &c, nil
}

func (s *CrawlStore) UpdateStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("UPDATE crawls SET status=?, updated_at=? WHERE id=?", status, now, id)
	return err
}

func (s *CrawlStore) UpdateProgress(id string, progress CrawlProgress) error {
	pJSON, _ := json.Marshal(progress)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("UPDATE crawls SET progress=?, updated_at=? WHERE id=?", string(pJSON), now, id)
	return err
}

func (s *CrawlStore) UpdateGraph(id string, graph CrawlGraph, requests []CapturedRequest) error {
	gJSON, _ := json.Marshal(graph)
	rJSON, _ := json.Marshal(requests)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("UPDATE crawls SET graph=?, requests=?, updated_at=? WHERE id=?", string(gJSON), string(rJSON), now, id)
	return err
}

func (s *CrawlStore) UpdateOpenAPISpec(id, spec string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("UPDATE crawls SET openapi_spec=?, updated_at=? WHERE id=?", spec, now, id)
	return err
}

func (s *CrawlStore) SetGeneratedPlan(id, planID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("UPDATE crawls SET generated_plan_id=?, updated_at=? WHERE id=?", planID, now, id)
	return err
}

func (s *CrawlStore) SetError(id, msg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("UPDATE crawls SET status='failed', error_message=?, updated_at=? WHERE id=?", msg, now, id)
	return err
}
