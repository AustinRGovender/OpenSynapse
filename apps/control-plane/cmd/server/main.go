package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
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

	planStore := db.NewPlanStore(database)
	envStore := db.NewEnvironmentStore(database)

	planHandlers := handlers.NewPlanHandlers(planStore)
	envHandlers := handlers.NewEnvironmentHandlers(envStore)

	r := router.New(planHandlers, envHandlers)

	fmt.Printf("OpenSynapse control plane listening on :%s\n", port)
	fmt.Printf("Database: %s\n", dbPath)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
