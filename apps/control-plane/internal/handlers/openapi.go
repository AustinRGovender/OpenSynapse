package handlers

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/auth"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/importer"
)

// The OpenAPI document is generated from this catalogue and the Go types
// the handlers actually decode and encode, so it cannot drift from the code:
// TestOpenAPICoversEveryRoute fails when a route is added without an entry.
// The TypeScript client in packages/api-client is generated from it
// (`pnpm --filter @opensynapse/api-client generate`).

// Op documents one API operation.
type Op struct {
	Method, Path, Summary, Tag string
	Request                    any    // body type (nil: no body)
	Response                   any    // 2xx body type (nil: no content)
	Status                     int    // success status (default 200)
	ContentType                string // non-JSON response type
	Query                      []string
	Public                     bool
}

// --- documentation-only shapes for responses built from maps ---

type items[T any] struct {
	Items []T `json:"items"`
}

type validateResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

type statusResponse struct {
	Status string `json:"status"`
}

type importJMXResponse struct {
	Plan db.Plan            `json:"plan"`
	Log  importer.ImportLog `json:"log"`
}

type exportRequestDoc struct {
	Format string `json:"format" enum:"json,csv,html,pdf"`
}

type loginResponse struct {
	Token     string         `json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	User      auth.Principal `json:"user"`
}

type createdToken struct {
	Token    string      `json:"token"`
	APIToken db.APIToken `json:"api_token"`
}

type passwordChange struct {
	Current string `json:"current_password"`
	New     string `json:"new_password"`
}

type tokenName struct {
	Name string `json:"name"`
}

type analyseResponse = db.AIAnalysis

type promptPreview struct {
	Prompt   string `json:"prompt"`
	Question string `json:"question"`
}

type keyTest struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

type aiConfigUpdate struct {
	Provider      string   `json:"provider"`
	APIKey        string   `json:"api_key,omitempty"`
	Model         string   `json:"model"`
	MonthlyCap    *float64 `json:"monthly_cap_usd"`
	Enabled       bool     `json:"enabled"`
	AzureEndpoint string   `json:"azure_endpoint,omitempty"`
}

type fragmentRequest struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
	NodeSubtree db.Node      `json:"node_subtree"`
	Bindings    []db.Binding `json:"bindings,omitempty"`
}

type reportRequest struct {
	Name          string   `json:"name"`
	RunIDs        []string `json:"run_ids"`
	Metrics       []string `json:"metrics,omitempty"`
	Normalisation string   `json:"normalisation,omitempty"`
}

type metricsPayload struct {
	Points []db.MetricPoint `json:"points"`
	State  engine.LiveState `json:"state"`
}

type oauthStartRequest struct {
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type oauthStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
	State        string `json:"state"`
	RedirectURI  string `json:"redirect_uri"`
}

type clientCredentialsRequest struct {
	TokenURL     string `json:"token_url"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type storageInfo struct {
	DataDir       string `json:"data_dir"`
	Database      string `json:"database"`
	DatabaseBytes int64  `json:"database_bytes"`
	SamplesBytes  int64  `json:"samples_bytes"`
	Runs          int    `json:"runs"`
	Retention     struct {
		RetentionMode  string `json:"retention_mode"`
		RetentionCount int    `json:"retention_count"`
		RetentionDays  int    `json:"retention_days"`
	} `json:"retention"`
	PrunableRuns     int   `json:"prunable_runs"`
	PrunableBytesEst int64 `json:"prunable_bytes_est"`
}

type cleanupResponse struct {
	DeletedRuns int `json:"deleted_runs"`
}

type logsResponse struct {
	Lines []string `json:"lines"`
}

type noteRequest struct {
	Type    string         `json:"type,omitempty" enum:"user_note"`
	Payload map[string]any `json:"payload"`
}

type versionResponse struct {
	Version string `json:"version"`
}

// ErrorResponse is every error body (docs/04 section 4).
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// Catalogue lists every operation the API serves.
var Catalogue = []Op{
	{"GET", "/health", "Liveness probe", "System", nil, statusResponse{}, 0, "", nil, true},
	{"GET", "/ready", "Readiness probe", "System", nil, statusResponse{}, 0, "", nil, true},
	{"GET", "/version", "Build version", "System", nil, versionResponse{}, 0, "", nil, true},
	{"GET", "/metrics", "Prometheus metrics for the control plane", "System", nil, nil, 0, "text/plain", nil, true},
	{"GET", "/openapi.json", "This OpenAPI document", "System", nil, nil, 0, "application/json", nil, true},
	{"GET", "/api/v1/system/info", "Installation and engine capabilities", "System", nil, SystemInfo{}, 0, "", nil, false},
	{"GET", "/api/v1/system/logs", "Recent control-plane log lines (admin)", "System", nil, logsResponse{}, 0, "", nil, false},

	{"GET", "/api/v1/auth/session", "How to authenticate; hands the UI the install token in local mode", "Auth", nil, sessionResponse{}, 0, "", nil, true},
	{"POST", "/api/v1/auth/setup", "Create the first admin (team mode)", "Auth", credentials{}, loginResponse{}, 201, "", nil, true},
	{"POST", "/api/v1/auth/login", "Sign in (team mode)", "Auth", credentials{}, loginResponse{}, 0, "", nil, true},
	{"POST", "/api/v1/auth/logout", "Sign out", "Auth", nil, nil, 204, "", nil, false},
	{"GET", "/api/v1/auth/me", "Current user", "Auth", nil, auth.Principal{}, 0, "", nil, false},
	{"PUT", "/api/v1/auth/password", "Change your password", "Auth", passwordChange{}, nil, 204, "", nil, false},
	{"GET", "/api/v1/auth/tokens", "List your API tokens", "Auth", nil, items[db.APIToken]{}, 0, "", nil, false},
	{"POST", "/api/v1/auth/tokens", "Create an API token (shown once)", "Auth", tokenName{}, createdToken{}, 201, "", nil, false},
	{"DELETE", "/api/v1/auth/tokens/{id}", "Revoke an API token", "Auth", nil, nil, 204, "", nil, false},
	{"GET", "/api/v1/users", "List users (admin)", "Users", nil, items[db.User]{}, 0, "", nil, false},
	{"POST", "/api/v1/users", "Create a user (admin)", "Users", userRequest{}, db.User{}, 201, "", nil, false},
	{"PUT", "/api/v1/users/{id}", "Update a user (admin)", "Users", userRequest{}, db.User{}, 0, "", nil, false},
	{"DELETE", "/api/v1/users/{id}", "Delete a user (admin)", "Users", nil, nil, 204, "", nil, false},

	{"GET", "/api/v1/settings", "Get settings", "Settings", nil, db.Settings{}, 0, "", nil, false},
	{"PUT", "/api/v1/settings", "Update settings (admin)", "Settings", db.Settings{}, db.Settings{}, 0, "", nil, false},
	{"GET", "/api/v1/storage", "Storage usage and retention preview", "Settings", nil, storageInfo{}, 0, "", nil, false},
	{"POST", "/api/v1/storage/cleanup", "Apply the retention policy now (admin)", "Settings", nil, cleanupResponse{}, 0, "", nil, false},
	{"POST", "/api/v1/diagnostics", "Download a diagnostics bundle without secrets (admin)", "Settings", nil, nil, 0, "application/zip", nil, false},

	{"GET", "/api/v1/plans", "List plans", "Plans", nil, db.ListResult[db.Plan]{}, 0, "", []string{"limit", "cursor"}, false},
	{"POST", "/api/v1/plans", "Create a plan", "Plans", createPlanRequest{}, db.Plan{}, 201, "", nil, false},
	{"GET", "/api/v1/plans/{id}", "Get a plan", "Plans", nil, db.Plan{}, 0, "", nil, false},
	{"PUT", "/api/v1/plans/{id}", "Replace a plan (creates a new version)", "Plans", updatePlanRequest{}, db.Plan{}, 0, "", nil, false},
	{"DELETE", "/api/v1/plans/{id}", "Delete a plan (built-in plans cannot be deleted)", "Plans", nil, nil, 204, "", nil, false},
	{"GET", "/api/v1/plans/{id}/versions", "List versions", "Plans", nil, items[db.PlanVersion]{}, 0, "", nil, false},
	{"GET", "/api/v1/plans/{id}/versions/{version}", "Get a version", "Plans", nil, db.PlanVersion{}, 0, "", nil, false},
	{"POST", "/api/v1/plans/{id}/validate", "Validate a plan without saving", "Plans", createPlanRequest{}, validateResponse{}, 0, "", nil, false},
	{"POST", "/api/v1/plans/{id}/compile", "Generate the k6 script (mode=managed for the script OpenSynapse runs)", "Plans", nil, nil, 0, "text/javascript", []string{"mode"}, false},
	{"POST", "/api/v1/plans/import/jmx", "Import a JMeter .jmx file (multipart field 'file' or raw XML)", "Plans", nil, importJMXResponse{}, 201, "", nil, false},

	{"GET", "/api/v1/environments", "List environments (secrets redacted)", "Environments", nil, db.ListResult[db.Environment]{}, 0, "", []string{"limit", "cursor"}, false},
	{"POST", "/api/v1/environments", "Create an environment", "Environments", createEnvironmentRequest{}, db.Environment{}, 201, "", nil, false},
	{"GET", "/api/v1/environments/{id}", "Get an environment", "Environments", nil, db.Environment{}, 0, "", nil, false},
	{"PUT", "/api/v1/environments/{id}", "Update an environment (send ******** to keep a secret)", "Environments", updateEnvironmentRequest{}, db.Environment{}, 0, "", nil, false},
	{"DELETE", "/api/v1/environments/{id}", "Delete an environment", "Environments", nil, nil, 204, "", nil, false},

	{"GET", "/api/v1/runs", "List runs", "Runs", nil, db.ListResult[db.Run]{}, 0, "", []string{"limit", "cursor", "plan_id", "status", "since", "until"}, false},
	{"POST", "/api/v1/runs", "Start a run", "Runs", startRunRequest{}, db.Run{}, 201, "", nil, false},
	{"GET", "/api/v1/runs/{id}", "Get a run", "Runs", nil, db.Run{}, 0, "", nil, false},
	{"DELETE", "/api/v1/runs/{id}", "Delete a run and its samples", "Runs", nil, nil, 204, "", nil, false},
	{"GET", "/api/v1/runs/{id}/samples", "Per-second metric series", "Runs", nil, items[db.MetricPoint]{}, 0, "", []string{"since", "until"}, false},
	{"GET", "/api/v1/runs/{id}/errors", "Recent failed requests", "Runs", nil, items[db.RunError]{}, 0, "", []string{"limit"}, false},
	{"GET", "/api/v1/runs/{id}/report", "Full report document", "Runs", nil, RunReport{}, 0, "", nil, false},
	{"GET", "/api/v1/runs/{id}/live", "Live-control state of a running test", "Runs", nil, engine.LiveState{}, 0, "", nil, false},
	{"GET", "/api/v1/runs/{id}/events", "Run timeline events", "Runs", nil, items[db.RunEvent]{}, 0, "", nil, false},
	{"POST", "/api/v1/runs/{id}/events", "Add a note to the timeline", "Runs", noteRequest{}, db.RunEvent{}, 201, "", nil, false},
	{"PATCH", "/api/v1/runs/{id}/control", "Change VUs, RPS limit, duration or pause", "Runs", controlRequest{}, engine.LiveState{}, 0, "", nil, false},
	{"GET", "/api/v1/runs/{id}/control-state", "Live-control state for the run's k6 control scenario (per-run token)", "Runs", nil, engine.ControlState{}, 0, "", nil, true},
	{"POST", "/api/v1/runs/{id}/stop", "Stop gracefully", "Runs", nil, statusResponse{}, 0, "", nil, false},
	{"POST", "/api/v1/runs/{id}/kill", "Kill immediately", "Runs", nil, statusResponse{}, 0, "", nil, false},
	{"POST", "/api/v1/runs/{id}/export", "Export as json, csv (raw samples), html or pdf", "Runs", exportRequestDoc{}, nil, 0, "application/octet-stream", nil, false},

	{"POST", "/api/v1/compare", "Compare 2–5 runs against the first", "Reports", compareRequest{}, comparisonResult{}, 0, "", nil, false},
	{"GET", "/api/v1/reports", "List saved comparisons", "Reports", nil, db.ListResult[db.Report]{}, 0, "", []string{"limit"}, false},
	{"POST", "/api/v1/reports", "Save a comparison", "Reports", reportRequest{}, db.Report{}, 201, "", nil, false},
	{"GET", "/api/v1/reports/{id}", "Get a saved comparison", "Reports", nil, db.Report{}, 0, "", nil, false},
	{"DELETE", "/api/v1/reports/{id}", "Delete a saved comparison", "Reports", nil, nil, 204, "", nil, false},

	{"POST", "/api/v1/playground/request", "Send one request with timing breakdown", "Playground", PlaygroundRequest{}, PlaygroundResponse{}, 0, "", nil, false},
	{"POST", "/api/v1/playground/microtest", "Run the request with up to 50 VUs for up to 60s", "Playground", microTestRequest{}, MicroTestResult{}, 0, "", nil, false},
	{"GET", "/api/v1/playground/collections", "List collections", "Playground", nil, db.ListResult[db.Collection]{}, 0, "", nil, false},
	{"POST", "/api/v1/playground/collections", "Create a collection", "Playground", collectionRequest{}, db.Collection{}, 201, "", nil, false},
	{"PUT", "/api/v1/playground/collections/{id}", "Update a collection", "Playground", collectionRequest{}, db.Collection{}, 0, "", nil, false},
	{"DELETE", "/api/v1/playground/collections/{id}", "Delete a collection", "Playground", nil, nil, 204, "", nil, false},
	{"POST", "/api/v1/playground/oauth/start", "Start an OAuth 2.0 authorization-code flow (PKCE)", "Playground", oauthStartRequest{}, oauthStartResponse{}, 0, "", nil, false},
	{"GET", "/api/v1/playground/oauth/callback", "OAuth redirect target (opened by the provider)", "Playground", nil, nil, 0, "text/html", []string{"code", "state", "error"}, true},
	{"GET", "/api/v1/playground/oauth/result", "Fetch the token from a finished OAuth flow", "Playground", nil, oauthToken{}, 0, "", []string{"state"}, false},
	{"POST", "/api/v1/playground/oauth/client-credentials", "Fetch a token with the client-credentials grant", "Playground", clientCredentialsRequest{}, oauthToken{}, 0, "", nil, false},

	{"POST", "/api/v1/crawls", "Start a crawl or OpenAPI import", "Crawler", startCrawlRequest{}, db.Crawl{}, 201, "", nil, false},
	{"GET", "/api/v1/crawls/{id}", "Crawl status and results", "Crawler", nil, db.Crawl{}, 0, "", nil, false},
	{"GET", "/api/v1/crawls/{id}/graph", "Crawl graph", "Crawler", nil, db.CrawlGraph{}, 0, "", nil, false},
	{"POST", "/api/v1/crawls/{id}/generate-plan", "Generate a draft plan", "Crawler", nil, db.Plan{}, 201, "", nil, false},
	{"POST", "/api/v1/crawls/{id}/cancel", "Cancel a crawl", "Crawler", nil, statusResponse{}, 0, "", nil, false},

	{"GET", "/api/v1/ai/config", "AI configuration (key masked)", "AI", nil, db.AIConfig{}, 0, "", nil, false},
	{"PUT", "/api/v1/ai/config", "Update AI configuration (admin)", "AI", aiConfigUpdate{}, db.AIConfig{}, 0, "", nil, false},
	{"POST", "/api/v1/ai/config/test", "Validate the configured key (sends one minimal request to the provider)", "AI", nil, keyTest{}, 0, "", nil, false},
	{"POST", "/api/v1/ai/preview", "Preview exactly what Analyse would send", "AI", analyseRequest{}, promptPreview{}, 0, "", nil, false},
	{"POST", "/api/v1/ai/analyse", "Analyse a run or comparison (sends data to the provider)", "AI", analyseRequest{}, analyseResponse{}, 0, "", nil, false},

	{"GET", "/api/v1/fragments", "List fragments", "Fragments", nil, db.ListResult[db.Fragment]{}, 0, "", nil, false},
	{"POST", "/api/v1/fragments", "Save a fragment", "Fragments", fragmentRequest{}, db.Fragment{}, 201, "", nil, false},
	{"GET", "/api/v1/fragments/{id}", "Get a fragment", "Fragments", nil, db.Fragment{}, 0, "", nil, false},
	{"PUT", "/api/v1/fragments/{id}", "Update a fragment (built-ins cannot change)", "Fragments", fragmentRequest{}, db.Fragment{}, 0, "", nil, false},
	{"DELETE", "/api/v1/fragments/{id}", "Delete a fragment (built-ins cannot be deleted)", "Fragments", nil, nil, 204, "", nil, false},

	{"GET", "/api/v1/ws", "WebSocket: send {type:auth,payload:{token}}, then subscribe to runs.{id}.metrics|events|status|errors", "Realtime", nil, metricsPayload{}, 101, "", nil, true},
}

// --- schema generation ---

type gen struct {
	schemas map[string]any
}

var genericName = regexp.MustCompile(`[^A-Za-z0-9]+`)

func schemaName(t reflect.Type) string {
	n := t.Name()
	if i := strings.Index(n, "["); i >= 0 {
		inner := n[i+1 : len(n)-1]
		if j := strings.LastIndex(inner, "."); j >= 0 {
			inner = inner[j+1:]
		}
		n = n[:i] + "_" + inner
	}
	n = genericName.ReplaceAllString(n, "")
	if len(n) > 0 && n[0] >= 'a' && n[0] <= 'z' {
		n = strings.ToUpper(n[:1]) + n[1:]
	}
	return n
}

var (
	timeType = reflect.TypeOf(time.Time{})
	rawType  = reflect.TypeOf(json.RawMessage{})
)

func (g *gen) schema(t reflect.Type) map[string]any {
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t == rawType:
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.schema(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}
		}
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		name := schemaName(t)
		if name == "" {
			return g.structSchema(t)
		}
		if _, ok := g.schemas[name]; !ok {
			g.schemas[name] = map[string]any{} // placeholder breaks recursion (Node.children)
			g.schemas[name] = g.structSchema(t)
		}
		return map[string]any{"$ref": "#/components/schemas/" + name}
	}
	return map[string]any{}
}

func (g *gen) structSchema(t reflect.Type) map[string]any {
	props := map[string]any{}
	var required []string
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Tag.Get("json") == "" {
				walk(f.Type)
				continue
			}
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" {
				name = f.Name
			}
			s := g.schema(f.Type)
			if enum := f.Tag.Get("enum"); enum != "" {
				s["enum"] = strings.Split(enum, ",")
			}
			if f.Type.Kind() == reflect.Pointer {
				if _, isRef := s["$ref"]; isRef {
					s = map[string]any{"anyOf": []any{s, map[string]any{"type": "null"}}}
				} else if typ, ok := s["type"].(string); ok {
					s["type"] = []any{typ, "null"}
				}
			}
			props[name] = s
			if !strings.Contains(opts, "omitempty") && f.Type.Kind() != reflect.Pointer {
				required = append(required, name)
			}
		}
	}
	walk(t)
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		out["required"] = required
	}
	return out
}

var pathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

// Document builds the OpenAPI 3.1 document.
func Document() map[string]any {
	g := &gen{schemas: map[string]any{}}
	g.schema(reflect.TypeOf(ErrorResponse{}))
	paths := map[string]any{}
	for _, op := range Catalogue {
		item, _ := paths[op.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[op.Path] = item
		}
		o := map[string]any{
			"summary":     op.Summary,
			"tags":        []string{op.Tag},
			"operationId": operationID(op),
		}
		var params []any
		for _, m := range pathParam.FindAllStringSubmatch(op.Path, -1) {
			params = append(params, map[string]any{"name": m[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
		}
		for _, q := range op.Query {
			params = append(params, map[string]any{"name": q, "in": "query", "required": false, "schema": map[string]any{"type": "string"}})
		}
		if len(params) > 0 {
			o["parameters"] = params
		}
		if op.Request != nil {
			o["requestBody"] = map[string]any{"required": true, "content": map[string]any{
				"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(op.Request))}}}
		}
		status := op.Status
		if status == 0 {
			status = 200
		}
		resp := map[string]any{"description": "Success"}
		switch {
		case op.ContentType != "":
			resp["content"] = map[string]any{op.ContentType: map[string]any{"schema": map[string]any{"type": "string"}}}
		case op.Response != nil:
			resp["content"] = map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(op.Response))}}
		}
		o["responses"] = map[string]any{
			itoa(status): resp,
			"default": map[string]any{"description": "Error", "content": map[string]any{
				"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ErrorResponse"}}}},
		}
		if op.Public {
			o["security"] = []any{}
		}
		item[strings.ToLower(op.Method)] = o
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "OpenSynapse API",
			"version":     BuildVersion,
			"description": "REST API of the OpenSynapse control plane. Authenticate with `Authorization: Bearer <token>`: the installation token in local mode (Settings › About), or a session or personal API token in team mode.",
		},
		"servers":    []any{map[string]any{"url": "/"}},
		"security":   []any{map[string]any{"bearerAuth": []any{}}},
		"paths":      paths,
		"components": map[string]any{"schemas": g.schemas, "securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"}}},
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func operationID(op Op) string {
	parts := []string{strings.ToLower(op.Method)}
	for _, seg := range strings.Split(strings.TrimPrefix(op.Path, "/api/v1"), "/") {
		seg = strings.Trim(seg, "{}")
		if seg == "" {
			continue
		}
		for _, w := range strings.FieldsFunc(seg, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
			parts = append(parts, strings.ToUpper(w[:1])+w[1:])
		}
	}
	return strings.Join(parts, "")
}

var (
	openAPIOnce sync.Once
	openAPIJSON []byte
)

// OpenAPI serves the generated document.
func OpenAPI(w http.ResponseWriter, r *http.Request) {
	openAPIOnce.Do(func() { openAPIJSON, _ = json.MarshalIndent(Document(), "", "  ") })
	w.Header().Set("Content-Type", "application/json")
	w.Write(openAPIJSON)
}
