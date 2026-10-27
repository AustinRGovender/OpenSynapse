package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/config"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/httpx"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	handlers.BuildVersion = version

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Stores
	planStore := db.NewPlanStore(database)
	envStore := db.NewEnvironmentStore(database)
	runStore := db.NewRunStore(database)
	reportStore := db.NewReportStore(database)
	collectionStore := db.NewCollectionStore(database)
	crawlStore := db.NewCrawlStore(database)
	aiStore := db.NewAIStore(database)
	fragmentStore := db.NewFragmentStore(database)

	if n, err := runStore.MarkInterrupted(); err != nil {
		log.Printf("WARNING: could not recover interrupted runs: %v", err)
	} else if n > 0 {
		log.Printf("Marked %d run(s) interrupted by the previous shutdown as failed", n)
	}

	// Built-in content: fragments, the test-API environments, and seed plans
	// that target those environments.
	fragmentStore.SeedBuiltInFragments()
	envIDs, err := envStore.SeedTestAPIEnvironments()
	if err != nil {
		log.Printf("WARNING: seeding test-API environments: %v", err)
	}
	planStore.SeedBuiltInPlans()
	defaultEnv := db.TestAPIsLocalEnv
	if os.Getenv("OPENSYNAPSE_TEST_APIS") == "docker" {
		defaultEnv = db.TestAPIsDockerEnv
	}
	if id := envIDs[defaultEnv]; id != "" {
		planStore.AssignBuiltInEnvironment(id)
	}

	// Engine
	eng, err := engine.New()
	if err != nil {
		log.Printf("WARNING: k6 engine not available: %v", err)
		log.Printf("Run execution will be disabled. Install k6 to enable.")
	}

	security := httpx.SecurityConfig{AllowedHosts: cfg.AllowedHosts, AllowedOrigins: cfg.AllowedOrigins}

	ws := wsserver.New()
	ws.OriginPatterns = security.OriginPatterns()

	runHandlers := handlers.NewRunHandlers(runStore, planStore, envStore, eng, ws)
	runHandlers.ControlBaseURL = cfg.ControlURL
	runHandlers.DataDir = cfg.DataDir

	deps := router.Deps{
		Plans:      handlers.NewPlanHandlers(planStore),
		Envs:       handlers.NewEnvironmentHandlers(envStore),
		Runs:       runHandlers,
		Reports:    handlers.NewReportHandlers(reportStore, runStore),
		Exports:    handlers.NewExportHandlers(runHandlers),
		Playground: handlers.NewPlaygroundHandlers(collectionStore),
		Crawls:     handlers.NewCrawlHandlers(crawlStore, planStore),
		AI:         handlers.NewAIHandlers(aiStore, runStore, reportStore),
		Fragments:  handlers.NewFragmentHandlers(fragmentStore),
		Imports:    handlers.NewImportHandlers(planStore),
		System: &handlers.SystemHandlers{
			Engine: eng, WS: ws, DataDir: cfg.DataDir, DBPath: cfg.DBPath,
			Mode: string(cfg.Mode), TeamMode: cfg.TeamMode,
		},
		WS:         ws,
		Middleware: []func(http.Handler) http.Handler{httpx.Middleware(security)},
	}

	srv := &http.Server{
		Addr:              cfg.Bind + ":" + cfg.Port,
		Handler:           router.New(deps),
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("OpenSynapse control plane %s listening on %s\n", version, srv.Addr)
	fmt.Printf("Data directory: %s\n", cfg.DataDir)
	if eng != nil {
		caps := eng.Capabilities()
		fmt.Printf("k6 engine: %s (xk6-kv: %v)\n", caps.Version, caps.KV)
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
