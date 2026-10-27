package testutil

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/auth"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/httpx"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// NewTestDB creates an in-memory SQLite database with migrations applied.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// TestServer holds a test HTTP server and its stores.
type TestServer struct {
	Server *httptest.Server
	DB     *sql.DB
	Plans  *db.PlanStore
	Envs   *db.EnvironmentStore
	Runs   *db.RunStore
	RunAPI *handlers.RunHandlers
	Users  *db.UserStore
	// Token is sent automatically by Do when a request has no Authorization header.
	Token string
}

// LocalToken is the installation token test servers use in local mode.
const LocalToken = "osl_test-token"

// Options customise a test server.
type Options struct {
	// Engine enables real runs (nil: run endpoints report ENGINE_NOT_AVAILABLE).
	Engine *engine.Engine
	// DataDir receives raw run samples.
	DataDir string
	// TeamMode requires sign-in (no automatic token is sent).
	TeamMode bool
}

// NewTestServer creates a test server with an in-memory database and no engine.
func NewTestServer(t *testing.T) *TestServer {
	return NewTestServerWith(t, Options{})
}

// NewTestServerWith creates a test server with the given options.
func NewTestServerWith(t *testing.T, opts Options) *TestServer {
	t.Helper()
	database := NewTestDB(t)

	planStore := db.NewPlanStore(database)
	envStore := db.NewEnvironmentStore(database)
	runStore := db.NewRunStore(database)
	reportStore := db.NewReportStore(database)
	collectionStore := db.NewCollectionStore(database)
	crawlStore := db.NewCrawlStore(database)
	aiStore := db.NewAIStore(database)
	fragmentStore := db.NewFragmentStore(database)
	fragmentStore.SeedBuiltInFragments()
	planStore.SeedBuiltInPlans()

	users := db.NewUserStore(database)
	settingsStore := db.NewSettingsStore(database)
	authSvc := &auth.Service{TeamMode: opts.TeamMode, LocalToken: LocalToken, Users: users}

	ws := wsserver.New()
	ws.Authenticate = func(t string) bool { _, ok := authSvc.Authenticate(t); return ok }
	runHandlers := handlers.NewRunHandlers(runStore, planStore, envStore, opts.Engine, ws)
	runHandlers.DataDir = opts.DataDir

	playground := handlers.NewPlaygroundHandlers(collectionStore)
	playground.Envs = envStore
	playground.Engine = opts.Engine
	system := &handlers.SystemHandlers{Engine: opts.Engine, WS: ws, Mode: "desktop", DataDir: opts.DataDir}
	deps := router.Deps{
		Plans:      handlers.NewPlanHandlers(planStore),
		Envs:       handlers.NewEnvironmentHandlers(envStore),
		Runs:       runHandlers,
		Reports:    handlers.NewReportHandlers(reportStore, runStore),
		Exports:    handlers.NewExportHandlers(runHandlers),
		Playground: playground,
		Crawls:     handlers.NewCrawlHandlers(crawlStore, planStore),
		AI:         handlers.NewAIHandlers(aiStore, runStore, reportStore),
		Fragments:  handlers.NewFragmentHandlers(fragmentStore),
		Imports:    handlers.NewImportHandlers(planStore),
		System:     system,
		Auth:       handlers.NewAuthHandlers(authSvc, users),
		Settings:   handlers.NewSettingsHandlers(settingsStore, runStore, system),
		WS:         ws,
		Middleware: []func(http.Handler) http.Handler{httpx.Middleware(httpx.SecurityConfig{}), authSvc.Middleware},
	}

	srv := httptest.NewServer(router.New(deps))
	t.Cleanup(srv.Close)
	// k6's control scenario reaches the API over loopback, like in production.
	runHandlers.ControlBaseURL = srv.URL

	ts := &TestServer{
		Server: srv,
		DB:     database,
		Plans:  planStore,
		Envs:   envStore,
		Runs:   runStore,
		RunAPI: runHandlers,
		Users:  users,
	}
	if !opts.TeamMode {
		ts.Token = LocalToken
	}
	return ts
}

// URL returns the test server's base URL.
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// Do executes an HTTP request against the test server, adding the server's
// token when the request carries none.
func (ts *TestServer) Do(req *http.Request) *http.Response {
	if ts.Token != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+ts.Token)
	}
	resp, err := ts.Server.Client().Do(req)
	if err != nil {
		panic("test request failed: " + err.Error())
	}
	return resp
}
