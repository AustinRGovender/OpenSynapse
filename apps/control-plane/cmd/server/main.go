package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/artifacts"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/auth"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/config"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/httpx"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/kube"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/notify"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/observability"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/secrets"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// runNotifier fans run notifications out to webhooks and metrics.
type runNotifier struct{ hooks *notify.Webhooks }

func (n runNotifier) RunStarted(run *db.Run) { n.hooks.RunStarted(run) }
func (n runNotifier) RunFinished(run *db.Run) {
	observability.Default.RunFinished(run.Status)
	n.hooks.RunFinished(run)
}

func main() {
	observability.SetupLogging()
	handlers.BuildVersion = version

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration: %v", err)
	}

	// SQLite in the data directory, or Postgres when DB_DSN is set (cluster mode).
	var database *sql.DB
	dbDescription := cfg.DBPath
	dbSize := func() int64 { return fileSize(cfg.DBPath) + fileSize(cfg.DBPath+"-wal") }
	if cfg.DBDSN != "" {
		database, err = db.OpenPostgres(cfg.DBDSN)
		dbDescription = "postgres"
		dbSize = func() int64 { return db.PostgresSize(database) }
	} else {
		database, err = db.Open(cfg.DBPath)
	}
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Raw run samples: the data directory, or S3-compatible storage when
	// MINIO_ENDPOINT is set.
	var store artifacts.Store = artifacts.NewDir(cfg.DataDir)
	if cfg.MinIOEndpoint != "" {
		s3, err := artifacts.NewS3(context.Background(), artifacts.S3Config{
			Endpoint: cfg.MinIOEndpoint, Bucket: cfg.MinIOBucket,
			AccessKey: cfg.MinIOAccessKey, SecretKey: cfg.MinIOSecretKey, UseSSL: cfg.MinIOUseSSL,
		})
		if err != nil {
			log.Fatalf("object storage: %v", err)
		}
		store = s3
	}

	box, err := secrets.Load(cfg.DataDir)
	if err != nil {
		log.Fatalf("secret key: %v", err)
	}

	// Stores
	planStore := db.NewPlanStore(database)
	envStore := db.NewEnvironmentStore(database)
	envStore.UseSecrets(box)
	runStore := db.NewRunStore(database)
	reportStore := db.NewReportStore(database)
	collectionStore := db.NewCollectionStore(database)
	crawlStore := db.NewCrawlStore(database)
	aiStore := db.NewAIStore(database)
	aiStore.UseSecrets(box)
	fragmentStore := db.NewFragmentStore(database)
	userStore := db.NewUserStore(database)
	settingsStore := db.NewSettingsStore(database)
	settingsStore.UseSecrets(box)

	if n, err := runStore.MarkInterrupted(); err != nil {
		slog.Warn("could not recover interrupted runs", "error", err)
	} else if n > 0 {
		slog.Info("marked runs interrupted by the previous shutdown as failed", "count", n)
	}

	// Built-in content: fragments, the test-API environments, and seed plans
	// that target those environments.
	fragmentStore.SeedBuiltInFragments()
	envIDs, err := envStore.SeedTestAPIEnvironments()
	if err != nil {
		slog.Warn("seeding test-API environments", "error", err)
	}
	planStore.SeedBuiltInPlans()
	defaultEnv := db.TestAPIsLocalEnv
	if testAPIsInDocker() {
		defaultEnv = db.TestAPIsDockerEnv
	}
	if id := envIDs[defaultEnv]; id != "" {
		planStore.AssignBuiltInEnvironment(id)
	}

	// Authentication
	localToken, err := auth.LoadLocalToken(cfg.DataDir)
	if err != nil {
		log.Fatalf("api token: %v", err)
	}
	authSvc := &auth.Service{TeamMode: cfg.TeamMode, LocalToken: localToken, Users: userStore}
	if cfg.TeamMode {
		bootstrapAdmin(userStore)
	}

	// Engine
	eng, err := engine.New()
	if err != nil {
		slog.Warn("k6 engine not available; run execution is disabled until k6 is installed", "error", err)
	}
	if eng != nil && cfg.Mode == config.ModeCluster {
		kc, err := kube.InCluster()
		if err != nil {
			log.Fatalf("cluster mode: %v", err)
		}
		kcfg := engine.KubernetesConfig{
			Client:          kc,
			RunnerImage:     cfg.RunnerImage,
			ImagePullPolicy: cfg.RunnerPullPolicy,
			MaxWorkers:      cfg.MaxWorkers,
		}
		if cfg.RunnerResources != "" {
			if err := json.Unmarshal([]byte(cfg.RunnerResources), &kcfg.RunnerResources); err != nil {
				log.Fatalf("OPENSYNAPSE_RUNNER_RESOURCES: %v", err)
			}
		}
		eng.UseKubernetes(kcfg)
		if n, err := engine.CleanupOrphans(context.Background(), kc); err != nil {
			slog.Warn("could not remove TestRuns left by a previous control plane", "error", err)
		} else if n > 0 {
			slog.Info("removed TestRuns left by a previous control plane", "count", n)
		}
		slog.Info("cluster mode: runs execute as k6-operator TestRuns", "namespace", kc.Namespace(), "runner_image", cfg.RunnerImage, "max_workers", cfg.MaxWorkers)
	}

	security := httpx.SecurityConfig{AllowedHosts: cfg.AllowedHosts, AllowedOrigins: cfg.AllowedOrigins}

	ws := wsserver.New()
	ws.OriginPatterns = security.OriginPatterns()
	ws.Authenticate = func(token string) bool {
		_, ok := authSvc.Authenticate(token)
		return ok
	}

	runHandlers := handlers.NewRunHandlers(runStore, planStore, envStore, eng, ws)
	runHandlers.ControlBaseURL = cfg.ControlURL
	runHandlers.Artifacts = store
	runHandlers.Notifier = runNotifier{hooks: &notify.Webhooks{Settings: settingsStore, PublicURL: os.Getenv("OPENSYNAPSE_PUBLIC_URL")}}

	system := &handlers.SystemHandlers{
		Engine: eng, WS: ws, DataDir: cfg.DataDir,
		Database: dbDescription, DatabaseSize: dbSize, Artifacts: store,
		Mode: string(cfg.Mode), TeamMode: cfg.TeamMode,
	}
	settingsHandlers := handlers.NewSettingsHandlers(settingsStore, runStore, system)
	planHandlers := handlers.NewPlanHandlers(planStore)
	playgroundHandlers := handlers.NewPlaygroundHandlers(collectionStore)
	playgroundHandlers.Envs = envStore
	playgroundHandlers.Engine = eng
	planHandlers.Settings = settingsStore

	observability.Default.Gauge("opensynapse_websocket_clients", func() float64 { return float64(ws.ClientCount()) })
	observability.Default.Gauge("opensynapse_websocket_dropped_frames_total", func() float64 { return float64(ws.DroppedFrames()) })
	observability.Default.Gauge("opensynapse_runs_active", func() float64 {
		if eng == nil {
			return 0
		}
		return float64(eng.ActiveCount())
	})
	observability.Default.Gauge("opensynapse_engine_available", func() float64 {
		if eng == nil {
			return 0
		}
		return 1
	})

	deps := router.Deps{
		Plans:      planHandlers,
		Envs:       handlers.NewEnvironmentHandlers(envStore),
		Runs:       runHandlers,
		Reports:    handlers.NewReportHandlers(reportStore, runStore),
		Exports:    handlers.NewExportHandlers(runHandlers),
		Playground: playgroundHandlers,
		Crawls:     handlers.NewCrawlHandlers(crawlStore, planStore),
		AI:         handlers.NewAIHandlers(aiStore, runStore, reportStore),
		Fragments:  handlers.NewFragmentHandlers(fragmentStore),
		Imports:    handlers.NewImportHandlers(planStore),
		System:     system,
		Auth:       handlers.NewAuthHandlers(authSvc, userStore),
		Settings:   settingsHandlers,
		WS:         ws,
		Middleware: []func(http.Handler) http.Handler{httpx.Middleware(security), authSvc.Middleware},
	}

	srv := &http.Server{
		Addr:              cfg.Bind + ":" + cfg.Port,
		Handler:           router.New(deps),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Apply the run retention policy hourly.
	go func() {
		for range time.Tick(time.Hour) {
			if n, err := settingsHandlers.ApplyRetention(); err == nil && n > 0 {
				slog.Info("retention policy deleted runs", "count", n)
			}
		}
	}()

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		slog.Info("shutting down")
		if eng != nil {
			eng.Shutdown(10 * time.Second)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	attrs := []any{"version", version, "address", srv.Addr, "data_dir", cfg.DataDir, "team_mode", cfg.TeamMode}
	if eng != nil {
		caps := eng.Capabilities()
		attrs = append(attrs, "k6", caps.Version, "xk6_kv", caps.KV)
	}
	slog.Info("OpenSynapse control plane listening", attrs...)
	if !cfg.TeamMode && cfg.Bind != "127.0.0.1" && cfg.Bind != "localhost" {
		slog.Warn("listening beyond loopback in local mode: anyone who can reach this port can use OpenSynapse. Set TEAM_MODE=true to require sign-in.")
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

// bootstrapAdmin creates the first admin from OPENSYNAPSE_ADMIN_EMAIL and
// OPENSYNAPSE_ADMIN_PASSWORD when team mode starts with no users. Without
// them, the UI offers a one-time "create admin account" screen instead.
func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// testAPIsInDocker reports whether the built-in plans should target the test
// APIs by Docker service name. OPENSYNAPSE_TEST_APIS=docker|localhost decides;
// unset, a container that can resolve the echo-api service (as under
// user-testing/docker-compose.yml) counts as Docker. The lookup goes to
// Docker's embedded DNS only, since it runs only inside a container.
func testAPIsInDocker() bool {
	switch os.Getenv("OPENSYNAPSE_TEST_APIS") {
	case "docker":
		return true
	case "localhost":
		return false
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, "echo-api")
	return err == nil
}

func bootstrapAdmin(users *db.UserStore) {
	if n, err := users.Count(); err != nil || n > 0 {
		return
	}
	email := strings.ToLower(strings.TrimSpace(os.Getenv("OPENSYNAPSE_ADMIN_EMAIL")))
	password := os.Getenv("OPENSYNAPSE_ADMIN_PASSWORD")
	if email == "" || password == "" {
		slog.Info("team mode has no users yet; open the web UI to create the first admin account")
		return
	}
	if _, _, err := users.Create(email, "Administrator", password, auth.RoleAdmin); err != nil {
		slog.Error("creating bootstrap admin", "error", err)
		return
	}
	slog.Info("created bootstrap admin account", "email", email)
}
