package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("cannot determine home directory: %v", err)
		}
		dataDir := filepath.Join(home, ".opensynapse")
		os.MkdirAll(dataDir, 0755)
		dbPath = filepath.Join(dataDir, "opensynapse.db")
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Stores
	planStore := db.NewPlanStore(database)
	envStore := db.NewEnvironmentStore(database)
	runStore := db.NewRunStore(database)
	reportStore := db.NewReportStore(database)

	// Engine
	eng, err := engine.New()
	if err != nil {
		log.Printf("WARNING: k6 engine not available: %v", err)
		log.Printf("Run execution will be disabled. Install k6 to enable.")
	}

	// WebSocket server
	ws := wsserver.New()

	// Handlers
	planHandlers := handlers.NewPlanHandlers(planStore)
	envHandlers := handlers.NewEnvironmentHandlers(envStore)
	runHandlers := handlers.NewRunHandlers(runStore, planStore, eng, ws)
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
	planStore.SeedBuiltInPlans()
	importHandlers := handlers.NewImportHandlers(planStore)

	r := router.New(planHandlers, envHandlers, runHandlers, reportHandlers, exportHandlers, playgroundHandlers, crawlHandlers, aiHandlers, fragmentHandlers, importHandlers, ws)

	fmt.Printf("OpenSynapse control plane listening on :%s\n", port)
	fmt.Printf("Database: %s\n", dbPath)
	if eng != nil {
		fmt.Println("k6 engine: ready")
	}
	fmt.Printf("WebSocket: ws://localhost:%s/api/v1/ws\n", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
