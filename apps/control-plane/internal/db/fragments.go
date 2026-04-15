package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Binding struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	DefaultValue *string `json:"default_value,omitempty"`
	Required     bool    `json:"required"`
}

type Fragment struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	NodeSubtree Node      `json:"node_subtree"`
	Bindings    []Binding `json:"bindings"`
	BuiltIn     bool      `json:"built_in"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FragmentStore struct {
	db *sql.DB
}

func NewFragmentStore(db *sql.DB) *FragmentStore {
	return &FragmentStore{db: db}
}

func (s *FragmentStore) Create(name, description string, tags []string, subtree Node, bindings []Binding, builtIn bool) (*Fragment, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	if tags == nil { tags = []string{} }
	if bindings == nil { bindings = []Binding{} }

	tagsJSON, _ := json.Marshal(tags)
	subtreeJSON, _ := json.Marshal(subtree)
	bindingsJSON, _ := json.Marshal(bindings)
	nowStr := now.Format(time.RFC3339)
	builtInInt := 0
	if builtIn { builtInInt = 1 }

	_, err := s.db.Exec(
		`INSERT INTO fragments (id, name, description, tags, node_subtree, bindings, built_in, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, description, string(tagsJSON), string(subtreeJSON), string(bindingsJSON), builtInInt, nowStr, nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("insert fragment: %w", err)
	}

	return &Fragment{
		ID: id, Name: name, Description: description, Tags: tags,
		NodeSubtree: subtree, Bindings: bindings, BuiltIn: builtIn,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *FragmentStore) Get(id string) (*Fragment, error) {
	var f Fragment
	var tagsStr, subtreeStr, bindingsStr, createdAt, updatedAt string
	var builtInInt int

	err := s.db.QueryRow(
		`SELECT id, name, description, tags, node_subtree, bindings, built_in, created_at, updated_at
		 FROM fragments WHERE id = ?`, id,
	).Scan(&f.ID, &f.Name, &f.Description, &tagsStr, &subtreeStr, &bindingsStr, &builtInInt, &createdAt, &updatedAt)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return nil, fmt.Errorf("get fragment: %w", err) }

	json.Unmarshal([]byte(tagsStr), &f.Tags)
	json.Unmarshal([]byte(subtreeStr), &f.NodeSubtree)
	json.Unmarshal([]byte(bindingsStr), &f.Bindings)
	if f.Tags == nil { f.Tags = []string{} }
	if f.Bindings == nil { f.Bindings = []Binding{} }
	f.BuiltIn = builtInInt == 1
	f.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	f.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &f, nil
}

func (s *FragmentStore) List() (*ListResult[Fragment], error) {
	rows, err := s.db.Query(
		`SELECT id, name, description, tags, node_subtree, bindings, built_in, created_at, updated_at
		 FROM fragments ORDER BY built_in DESC, name ASC`,
	)
	if err != nil { return nil, fmt.Errorf("list fragments: %w", err) }
	defer rows.Close()

	var fragments []Fragment
	for rows.Next() {
		var f Fragment
		var tagsStr, subtreeStr, bindingsStr, createdAt, updatedAt string
		var builtInInt int
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &tagsStr, &subtreeStr, &bindingsStr, &builtInInt, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan fragment: %w", err)
		}
		json.Unmarshal([]byte(tagsStr), &f.Tags)
		json.Unmarshal([]byte(subtreeStr), &f.NodeSubtree)
		json.Unmarshal([]byte(bindingsStr), &f.Bindings)
		if f.Tags == nil { f.Tags = []string{} }
		if f.Bindings == nil { f.Bindings = []Binding{} }
		f.BuiltIn = builtInInt == 1
		f.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		f.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		fragments = append(fragments, f)
	}
	if fragments == nil { fragments = []Fragment{} }
	return &ListResult[Fragment]{Items: fragments}, nil
}

func (s *FragmentStore) Update(id, name, description string, tags []string, subtree Node, bindings []Binding) (*Fragment, error) {
	existing, err := s.Get(id)
	if err != nil { return nil, err }
	if existing == nil { return nil, nil }
	if existing.BuiltIn { return nil, fmt.Errorf("cannot modify built-in fragment") }

	now := time.Now().UTC()
	if tags == nil { tags = []string{} }
	if bindings == nil { bindings = []Binding{} }
	tagsJSON, _ := json.Marshal(tags)
	subtreeJSON, _ := json.Marshal(subtree)
	bindingsJSON, _ := json.Marshal(bindings)
	nowStr := now.Format(time.RFC3339)

	_, err = s.db.Exec(
		`UPDATE fragments SET name=?, description=?, tags=?, node_subtree=?, bindings=?, updated_at=? WHERE id=?`,
		name, description, string(tagsJSON), string(subtreeJSON), string(bindingsJSON), nowStr, id,
	)
	if err != nil { return nil, fmt.Errorf("update fragment: %w", err) }

	return &Fragment{
		ID: id, Name: name, Description: description, Tags: tags,
		NodeSubtree: subtree, Bindings: bindings, BuiltIn: false,
		CreatedAt: existing.CreatedAt, UpdatedAt: now,
	}, nil
}

func (s *FragmentStore) Delete(id string) error {
	// Check if built-in
	var builtIn int
	err := s.db.QueryRow("SELECT built_in FROM fragments WHERE id = ?", id).Scan(&builtIn)
	if err == sql.ErrNoRows { return fmt.Errorf("fragment not found") }
	if err != nil { return err }
	if builtIn == 1 { return fmt.Errorf("cannot delete built-in fragment") }

	_, err = s.db.Exec("DELETE FROM fragments WHERE id = ?", id)
	return err
}

// SeedBuiltInFragments inserts the shipped fragments if they don't exist.
func (s *FragmentStore) SeedBuiltInFragments() error {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM fragments WHERE built_in = 1").Scan(&count)
	if count > 0 { return nil }

	fragments := builtInFragments()
	for _, f := range fragments {
		s.Create(f.Name, f.Description, f.Tags, f.NodeSubtree, f.Bindings, true)
	}
	return nil
}

func builtInFragments() []Fragment {
	return []Fragment{
		{Name: "Generic Form Login", Description: "Login via HTML form with username and password fields", Tags: []string{"auth", "login"},
			NodeSubtree: Node{ID: "frag-login", Type: "once-only-controller", Name: "Login", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{ID: "frag-login-req", Type: "http", Name: "POST Login", Enabled: true,
					Properties: json.RawMessage(`{"method":"POST","url":"${BASE_URL}/login","headers":[{"key":"Content-Type","value":"application/x-www-form-urlencoded"}],"body":{"type":"form","content":"username=${USERNAME}&password=${PASSWORD}"},"follow_redirects":true}`),
					Children: []Node{}}}},
			Bindings: []Binding{{Name: "USERNAME", Description: "Login username", Required: true}, {Name: "PASSWORD", Description: "Login password", Required: true}}},
		{Name: "CSRF Token Extraction", Description: "Extract CSRF token from a page and use it in subsequent requests", Tags: []string{"auth", "csrf"},
			NodeSubtree: Node{ID: "frag-csrf", Type: "transaction-controller", Name: "CSRF Flow", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: []Node{{ID: "frag-csrf-get", Type: "http", Name: "GET Page with CSRF", Enabled: true,
					Properties: json.RawMessage(`{"method":"GET","url":"${BASE_URL}/${CSRF_PAGE}","headers":[],"body":{"type":"none"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "CSRF_PAGE", Description: "Page containing the CSRF token", Required: true}}},
		{Name: "Pagination Walker", Description: "Walk through paginated results", Tags: []string{"pagination"},
			NodeSubtree: Node{ID: "frag-page", Type: "loop-controller", Name: "Paginate", Enabled: true,
				Properties: json.RawMessage(`{"count":10}`),
				Children: []Node{{ID: "frag-page-req", Type: "http", Name: "GET Page", Enabled: true,
					Properties: json.RawMessage(`{"method":"GET","url":"${BASE_URL}/${ENDPOINT}?page=${__ITER}","headers":[],"body":{"type":"none"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "ENDPOINT", Description: "API endpoint to paginate", Required: true}}},
		{Name: "Search-then-Select", Description: "Search for items and select the first result", Tags: []string{"workflow"},
			NodeSubtree: Node{ID: "frag-search", Type: "transaction-controller", Name: "Search and Select", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: []Node{
					{ID: "frag-search-req", Type: "http", Name: "GET Search", Enabled: true,
						Properties: json.RawMessage(`{"method":"GET","url":"${BASE_URL}/search?q=${QUERY}","headers":[],"body":{"type":"none"}}`), Children: []Node{}},
					{ID: "frag-search-select", Type: "http", Name: "GET Detail", Enabled: true,
						Properties: json.RawMessage(`{"method":"GET","url":"${BASE_URL}/items/${ITEM_ID}","headers":[],"body":{"type":"none"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "QUERY", Description: "Search query", Required: true}, {Name: "ITEM_ID", Description: "Item ID to select", Required: true}}},
		{Name: "Cart Checkout", Description: "Add item to cart and checkout", Tags: []string{"ecommerce"},
			NodeSubtree: Node{ID: "frag-cart", Type: "transaction-controller", Name: "Checkout", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: []Node{
					{ID: "frag-cart-add", Type: "http", Name: "POST Add to Cart", Enabled: true,
						Properties: json.RawMessage(`{"method":"POST","url":"${BASE_URL}/cart","headers":[{"key":"Content-Type","value":"application/json"}],"body":{"type":"json","content":"{\"product_id\":\"${PRODUCT_ID}\",\"quantity\":1}"}}`), Children: []Node{}},
					{ID: "frag-cart-checkout", Type: "http", Name: "POST Checkout", Enabled: true,
						Properties: json.RawMessage(`{"method":"POST","url":"${BASE_URL}/checkout","headers":[{"key":"Content-Type","value":"application/json"}],"body":{"type":"json","content":"{}"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "PRODUCT_ID", Description: "Product to add to cart", Required: true}}},
		{Name: "File Upload", Description: "Upload a file via multipart form", Tags: []string{"file"},
			NodeSubtree: Node{ID: "frag-upload", Type: "http", Name: "POST Upload", Enabled: true,
				Properties: json.RawMessage(`{"method":"POST","url":"${BASE_URL}/${UPLOAD_PATH}","headers":[],"body":{"type":"multipart"},"follow_redirects":true}`), Children: []Node{}},
			Bindings: []Binding{{Name: "UPLOAD_PATH", Description: "Upload endpoint path", Required: true}}},
		{Name: "File Download with Hash", Description: "Download a file and verify its hash", Tags: []string{"file", "verification"},
			NodeSubtree: Node{ID: "frag-download", Type: "transaction-controller", Name: "Download and Verify", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: []Node{{ID: "frag-dl-req", Type: "http", Name: "GET Download", Enabled: true,
					Properties: json.RawMessage(`{"method":"GET","url":"${BASE_URL}/${DOWNLOAD_PATH}","headers":[],"body":{"type":"none"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "DOWNLOAD_PATH", Description: "File download path", Required: true}}},
		{Name: "OAuth Authorization Code", Description: "OAuth 2.0 authorization code flow", Tags: []string{"auth", "oauth"},
			NodeSubtree: Node{ID: "frag-oauth", Type: "once-only-controller", Name: "OAuth Flow", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: []Node{{ID: "frag-oauth-token", Type: "http", Name: "POST Token Exchange", Enabled: true,
					Properties: json.RawMessage(`{"method":"POST","url":"${TOKEN_URL}","headers":[{"key":"Content-Type","value":"application/x-www-form-urlencoded"}],"body":{"type":"form","content":"grant_type=authorization_code&code=${AUTH_CODE}&client_id=${CLIENT_ID}&client_secret=${CLIENT_SECRET}&redirect_uri=${REDIRECT_URI}"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "TOKEN_URL", Description: "Token endpoint URL", Required: true}, {Name: "CLIENT_ID", Description: "OAuth client ID", Required: true}, {Name: "CLIENT_SECRET", Description: "OAuth client secret", Required: true}}},
		{Name: "SAML Login Stub", Description: "SAML authentication stub (requires IdP configuration)", Tags: []string{"auth", "saml"},
			NodeSubtree: Node{ID: "frag-saml", Type: "once-only-controller", Name: "SAML Login", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: []Node{{ID: "frag-saml-req", Type: "http", Name: "POST SAML Assert", Enabled: true,
					Properties: json.RawMessage(`{"method":"POST","url":"${BASE_URL}/${SAML_ACS}","headers":[{"key":"Content-Type","value":"application/x-www-form-urlencoded"}],"body":{"type":"form","content":"SAMLResponse=${SAML_RESPONSE}"}}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "SAML_ACS", Description: "SAML Assertion Consumer Service path", Required: true}}},
		{Name: "Wait-for-Condition Polling", Description: "Poll an endpoint until a condition is met", Tags: []string{"polling", "async"},
			NodeSubtree: Node{ID: "frag-poll", Type: "loop-controller", Name: "Poll until ready", Enabled: true,
				Properties: json.RawMessage(`{"count":30}`),
				Children: []Node{
					{ID: "frag-poll-req", Type: "http", Name: "GET Status", Enabled: true,
						Properties: json.RawMessage(`{"method":"GET","url":"${BASE_URL}/${POLL_PATH}","headers":[],"body":{"type":"none"}}`), Children: []Node{}},
					{ID: "frag-poll-wait", Type: "timer", Name: "Wait", Enabled: true,
						Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":2000}`), Children: []Node{}}}},
			Bindings: []Binding{{Name: "POLL_PATH", Description: "Status endpoint to poll", Required: true}}},
	}
}
