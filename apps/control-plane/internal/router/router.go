package router

import (
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// New creates and returns the HTTP router with all routes registered.
func New(plans *handlers.PlanHandlers, envs *handlers.EnvironmentHandlers, runs *handlers.RunHandlers, reports *handlers.ReportHandlers, exports *handlers.ExportHandlers, playground *handlers.PlaygroundHandlers, crawls *handlers.CrawlHandlers, ws *wsserver.Server) http.Handler {
	mux := http.NewServeMux()

	// System endpoints
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready)
	mux.HandleFunc("GET /version", handlers.Version)

	// Plans API (section 2.1)
	mux.HandleFunc("GET /api/v1/plans", plans.List)
	mux.HandleFunc("POST /api/v1/plans", plans.Create)
	mux.HandleFunc("GET /api/v1/plans/{id}", plans.Get)
	mux.HandleFunc("PUT /api/v1/plans/{id}", plans.Update)
	mux.HandleFunc("DELETE /api/v1/plans/{id}", plans.Delete)
	mux.HandleFunc("GET /api/v1/plans/{id}/versions", plans.ListVersions)
	mux.HandleFunc("GET /api/v1/plans/{id}/versions/{version}", plans.GetVersion)
	mux.HandleFunc("POST /api/v1/plans/{id}/validate", plans.Validate)
	mux.HandleFunc("POST /api/v1/plans/{id}/compile", plans.Compile)

	// Environments API (section 2.2)
	mux.HandleFunc("GET /api/v1/environments", envs.List)
	mux.HandleFunc("POST /api/v1/environments", envs.Create)
	mux.HandleFunc("GET /api/v1/environments/{id}", envs.Get)
	mux.HandleFunc("PUT /api/v1/environments/{id}", envs.Update)
	mux.HandleFunc("DELETE /api/v1/environments/{id}", envs.Delete)

	// Runs API (section 2.3)
	mux.HandleFunc("GET /api/v1/runs", runs.List)
	mux.HandleFunc("POST /api/v1/runs", runs.Start)
	mux.HandleFunc("GET /api/v1/runs/{id}", runs.Get)
	mux.HandleFunc("DELETE /api/v1/runs/{id}", runs.Delete)
	mux.HandleFunc("GET /api/v1/runs/{id}/events", runs.ListEvents)
	mux.HandleFunc("POST /api/v1/runs/{id}/events", runs.AddEvent)
	mux.HandleFunc("PATCH /api/v1/runs/{id}/control", runs.Control)
	mux.HandleFunc("POST /api/v1/runs/{id}/stop", runs.Stop)
	mux.HandleFunc("POST /api/v1/runs/{id}/kill", runs.Kill)
	mux.HandleFunc("POST /api/v1/runs/{id}/export", exports.ExportRun)

	// Comparison & Reports API (section 2.4)
	mux.HandleFunc("POST /api/v1/compare", reports.Compare)
	mux.HandleFunc("GET /api/v1/reports", reports.List)
	mux.HandleFunc("POST /api/v1/reports", reports.Create)
	mux.HandleFunc("GET /api/v1/reports/{id}", reports.Get)
	mux.HandleFunc("DELETE /api/v1/reports/{id}", reports.Delete)

	// Playground API (section 2.8)
	mux.HandleFunc("POST /api/v1/playground/request", playground.ExecuteRequest)
	mux.HandleFunc("GET /api/v1/playground/collections", playground.ListCollections)
	mux.HandleFunc("POST /api/v1/playground/collections", playground.CreateCollection)

	// Crawler API (section 2.6)
	mux.HandleFunc("POST /api/v1/crawls", crawls.Start)
	mux.HandleFunc("GET /api/v1/crawls/{id}", crawls.Get)
	mux.HandleFunc("GET /api/v1/crawls/{id}/graph", crawls.GetGraph)
	mux.HandleFunc("POST /api/v1/crawls/{id}/generate-plan", crawls.GeneratePlan)
	mux.HandleFunc("POST /api/v1/crawls/{id}/cancel", crawls.Cancel)

	// WebSocket
	if ws != nil {
		mux.HandleFunc("/api/v1/ws", ws.HandleWS)
	}

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
