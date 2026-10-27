package router

import (
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/observability"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// Deps holds every handler group the router mounts.
type Deps struct {
	Plans      *handlers.PlanHandlers
	Envs       *handlers.EnvironmentHandlers
	Runs       *handlers.RunHandlers
	Reports    *handlers.ReportHandlers
	Exports    *handlers.ExportHandlers
	Playground *handlers.PlaygroundHandlers
	Crawls     *handlers.CrawlHandlers
	AI         *handlers.AIHandlers
	Fragments  *handlers.FragmentHandlers
	Imports    *handlers.ImportHandlers
	System     *handlers.SystemHandlers
	Auth       *handlers.AuthHandlers
	Settings   *handlers.SettingsHandlers
	WS         *wsserver.Server

	// Middleware wraps the whole mux (security, auth, logging, metrics).
	Middleware []func(http.Handler) http.Handler
}

// Routes is every route the API serves, as "METHOD /path" patterns. It is
// the source of truth the OpenAPI drift test checks against.
var Routes []string

// New creates and returns the HTTP router with all routes registered.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	Routes = Routes[:0]
	handle := func(pattern string, h http.HandlerFunc) {
		Routes = append(Routes, pattern)
		mux.HandleFunc(pattern, observability.Default.Instrument(pattern, h))
	}

	// System endpoints
	handle("GET /health", handlers.Health)
	handle("GET /ready", handlers.Ready)
	handle("GET /version", handlers.Version)
	handle("GET /metrics", observability.Default.Handler)
	handle("GET /openapi.json", handlers.OpenAPI)
	if d.System != nil {
		handle("GET /api/v1/system/info", d.System.Info)
	}

	// Authentication and accounts (docs/04 section 5)
	if d.Auth != nil {
		handle("GET /api/v1/auth/session", d.Auth.Session)
		handle("POST /api/v1/auth/setup", d.Auth.Setup)
		handle("POST /api/v1/auth/login", d.Auth.Login)
		handle("POST /api/v1/auth/logout", d.Auth.Logout)
		handle("GET /api/v1/auth/me", d.Auth.Me)
		handle("PUT /api/v1/auth/password", d.Auth.ChangePassword)
		handle("GET /api/v1/auth/tokens", d.Auth.ListTokens)
		handle("POST /api/v1/auth/tokens", d.Auth.CreateToken)
		handle("DELETE /api/v1/auth/tokens/{id}", d.Auth.DeleteToken)
		handle("GET /api/v1/users", d.Auth.ListUsers)
		handle("POST /api/v1/users", d.Auth.CreateUser)
		handle("PUT /api/v1/users/{id}", d.Auth.UpdateUser)
		handle("DELETE /api/v1/users/{id}", d.Auth.DeleteUser)
	}

	// Settings (F-11)
	if d.Settings != nil {
		handle("GET /api/v1/settings", d.Settings.Get)
		handle("PUT /api/v1/settings", d.Settings.Update)
		handle("GET /api/v1/storage", d.Settings.Storage)
		handle("POST /api/v1/storage/cleanup", d.Settings.Cleanup)
		handle("POST /api/v1/diagnostics", d.Settings.Diagnostics)
		handle("GET /api/v1/system/logs", d.Settings.Logs)
	}

	// Plans API (section 2.1)
	handle("GET /api/v1/plans", d.Plans.List)
	handle("POST /api/v1/plans", d.Plans.Create)
	handle("GET /api/v1/plans/{id}", d.Plans.Get)
	handle("PUT /api/v1/plans/{id}", d.Plans.Update)
	handle("DELETE /api/v1/plans/{id}", d.Plans.Delete)
	handle("GET /api/v1/plans/{id}/versions", d.Plans.ListVersions)
	handle("GET /api/v1/plans/{id}/versions/{version}", d.Plans.GetVersion)
	handle("POST /api/v1/plans/{id}/validate", d.Plans.Validate)
	handle("POST /api/v1/plans/{id}/compile", d.Plans.Compile)
	handle("POST /api/v1/plans/import/jmx", d.Imports.ImportJMX)

	// Environments API (section 2.2)
	handle("GET /api/v1/environments", d.Envs.List)
	handle("POST /api/v1/environments", d.Envs.Create)
	handle("GET /api/v1/environments/{id}", d.Envs.Get)
	handle("PUT /api/v1/environments/{id}", d.Envs.Update)
	handle("DELETE /api/v1/environments/{id}", d.Envs.Delete)

	// Runs API (section 2.3)
	handle("GET /api/v1/runs", d.Runs.List)
	handle("POST /api/v1/runs", d.Runs.Start)
	handle("GET /api/v1/runs/{id}", d.Runs.Get)
	handle("DELETE /api/v1/runs/{id}", d.Runs.Delete)
	handle("GET /api/v1/runs/{id}/samples", d.Runs.Samples)
	handle("GET /api/v1/runs/{id}/errors", d.Runs.Errors)
	handle("GET /api/v1/runs/{id}/report", d.Runs.Report)
	handle("GET /api/v1/runs/{id}/live", d.Runs.Live)
	handle("GET /api/v1/runs/{id}/events", d.Runs.ListEvents)
	handle("POST /api/v1/runs/{id}/events", d.Runs.AddEvent)
	handle("PATCH /api/v1/runs/{id}/control", d.Runs.Control)
	handle("GET /api/v1/runs/{id}/control-state", d.Runs.ControlState)
	handle("POST /api/v1/runs/{id}/ingest", d.Runs.Ingest)
	handle("POST /api/v1/runs/{id}/stop", d.Runs.Stop)
	handle("POST /api/v1/runs/{id}/kill", d.Runs.Kill)
	handle("POST /api/v1/runs/{id}/export", d.Exports.ExportRun)

	// Comparison & Reports API (section 2.4)
	handle("POST /api/v1/compare", d.Reports.Compare)
	handle("GET /api/v1/reports", d.Reports.List)
	handle("POST /api/v1/reports", d.Reports.Create)
	handle("GET /api/v1/reports/{id}", d.Reports.Get)
	handle("DELETE /api/v1/reports/{id}", d.Reports.Delete)

	// Playground API (section 2.8)
	handle("POST /api/v1/playground/request", d.Playground.ExecuteRequest)
	handle("GET /api/v1/playground/collections", d.Playground.ListCollections)
	handle("POST /api/v1/playground/collections", d.Playground.CreateCollection)
	handle("PUT /api/v1/playground/collections/{id}", d.Playground.UpdateCollection)
	handle("DELETE /api/v1/playground/collections/{id}", d.Playground.DeleteCollection)
	handle("POST /api/v1/playground/microtest", d.Playground.MicroTest)
	handle("POST /api/v1/playground/oauth/start", d.Playground.OAuthStart)
	handle("GET /api/v1/playground/oauth/callback", d.Playground.OAuthCallback)
	handle("GET /api/v1/playground/oauth/result", d.Playground.OAuthResult)
	handle("POST /api/v1/playground/oauth/client-credentials", d.Playground.OAuthClientCredentials)

	// Crawler API (section 2.6)
	handle("POST /api/v1/crawls", d.Crawls.Start)
	handle("GET /api/v1/crawls/{id}", d.Crawls.Get)
	handle("GET /api/v1/crawls/{id}/graph", d.Crawls.GetGraph)
	handle("POST /api/v1/crawls/{id}/generate-plan", d.Crawls.GeneratePlan)
	handle("POST /api/v1/crawls/{id}/cancel", d.Crawls.Cancel)

	// AI API (section 2.7)
	handle("GET /api/v1/ai/config", d.AI.GetConfig)
	handle("PUT /api/v1/ai/config", d.AI.UpdateConfig)
	handle("POST /api/v1/ai/config/test", d.AI.TestConfig)
	handle("POST /api/v1/ai/preview", d.AI.Preview)
	handle("POST /api/v1/ai/analyse", d.AI.Analyse)

	// Fragments API (section 2.5)
	handle("GET /api/v1/fragments", d.Fragments.List)
	handle("POST /api/v1/fragments", d.Fragments.Create)
	handle("GET /api/v1/fragments/{id}", d.Fragments.Get)
	handle("PUT /api/v1/fragments/{id}", d.Fragments.Update)
	handle("DELETE /api/v1/fragments/{id}", d.Fragments.Delete)

	// WebSocket
	if d.WS != nil {
		handle("GET /api/v1/ws", d.WS.HandleWS)
	}

	var h http.Handler = mux
	for i := len(d.Middleware) - 1; i >= 0; i-- {
		h = d.Middleware[i](h)
	}
	return h
}
