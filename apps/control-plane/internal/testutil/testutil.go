package testutil

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
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
	Plans  *db.PlanStore
	Envs   *db.EnvironmentStore
	Runs   *db.RunStore
}

// NewTestServer creates a test server with an in-memory database.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()
	database := NewTestDB(t)

	planStore := db.NewPlanStore(database)
	envStore := db.NewEnvironmentStore(database)
	runStore := db.NewRunStore(database)

	reportStore := db.NewReportStore(database)

	planHandlers := handlers.NewPlanHandlers(planStore)
	envHandlers := handlers.NewEnvironmentHandlers(envStore)
	runHandlers := handlers.NewRunHandlers(runStore, planStore, nil, nil)
	reportHandlers := handlers.NewReportHandlers(reportStore, runStore)
	exportHandlers := handlers.NewExportHandlers(runStore)
	collectionStore := db.NewCollectionStore(database)
	crawlStore := db.NewCrawlStore(database)
	playgroundHandlers := handlers.NewPlaygroundHandlers(collectionStore)
	crawlHandlers := handlers.NewCrawlHandlers(crawlStore, planStore)
	aiStore := db.NewAIStore(database)
	aiHandlers := handlers.NewAIHandlers(aiStore, runStore)
	fragmentStore := db.NewFragmentStore(database)
	fragmentStore.SeedBuiltInFragments()
	fragmentHandlers := handlers.NewFragmentHandlers(fragmentStore)
	importHandlers := handlers.NewImportHandlers(planStore)
	ws := wsserver.New()

	r := router.New(planHandlers, envHandlers, runHandlers, reportHandlers, exportHandlers, playgroundHandlers, crawlHandlers, aiHandlers, fragmentHandlers, importHandlers, ws)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &TestServer{
		Server: srv,
		Plans:  planStore,
		Envs:   envStore,
		Runs:   runStore,
	}
}

// URL returns the test server's base URL.
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// Do executes an HTTP request against the test server.
func (ts *TestServer) Do(req *http.Request) *http.Response {
	resp, err := ts.Server.Client().Do(req)
	if err != nil {
		panic("test request failed: " + err.Error())
	}
	return resp
}
