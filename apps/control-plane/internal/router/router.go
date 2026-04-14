package router

import (
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
)

// New creates and returns the HTTP router with all routes registered.
func New(plans *handlers.PlanHandlers, envs *handlers.EnvironmentHandlers) http.Handler {
	mux := http.NewServeMux()

	// System endpoints
	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.Ready)
	mux.HandleFunc("GET /version", handlers.Version)

	// Plans API (section 2.1 of data model doc)
	mux.HandleFunc("GET /api/v1/plans", plans.List)
	mux.HandleFunc("POST /api/v1/plans", plans.Create)
	mux.HandleFunc("GET /api/v1/plans/{id}", plans.Get)
	mux.HandleFunc("PUT /api/v1/plans/{id}", plans.Update)
	mux.HandleFunc("DELETE /api/v1/plans/{id}", plans.Delete)
	mux.HandleFunc("GET /api/v1/plans/{id}/versions", plans.ListVersions)
	mux.HandleFunc("GET /api/v1/plans/{id}/versions/{version}", plans.GetVersion)
	mux.HandleFunc("POST /api/v1/plans/{id}/validate", plans.Validate)
	mux.HandleFunc("POST /api/v1/plans/{id}/compile", plans.Compile)

	// Environments API (section 2.2 of data model doc)
	mux.HandleFunc("GET /api/v1/environments", envs.List)
	mux.HandleFunc("POST /api/v1/environments", envs.Create)
	mux.HandleFunc("GET /api/v1/environments/{id}", envs.Get)
	mux.HandleFunc("PUT /api/v1/environments/{id}", envs.Update)
	mux.HandleFunc("DELETE /api/v1/environments/{id}", envs.Delete)

	// CORS middleware for development
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
