package handlers

import (
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/compiler"
)

// Compile handles POST /api/v1/plans/{id}/compile.
// It reads the plan from the DB, compiles it to a k6 JavaScript file,
// and returns the script as text/javascript.
func (h *PlanHandlers) Compile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	plan, err := h.store.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if plan == nil {
		notFound(w, "PLAN", id)
		return
	}

	script, err := compiler.Compile(plan)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "COMPILE_ERROR",
			"Failed to compile plan to k6 script: "+err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}
