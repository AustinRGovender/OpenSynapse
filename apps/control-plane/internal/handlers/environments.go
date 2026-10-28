package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type EnvironmentHandlers struct {
	store *db.EnvironmentStore
}

func NewEnvironmentHandlers(store *db.EnvironmentStore) *EnvironmentHandlers {
	return &EnvironmentHandlers{store: store}
}

type createEnvironmentRequest struct {
	Name      string                 `json:"name"`
	Variables map[string]db.Variable `json:"variables,omitempty"`
}

type updateEnvironmentRequest struct {
	Name      string                 `json:"name"`
	Variables map[string]db.Variable `json:"variables"`
}

func (h *EnvironmentHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cursor := r.URL.Query().Get("cursor")

	result, err := h.store.List(db.ListParams{Limit: limit, Cursor: cursor})
	if err != nil {
		internalError(w, err)
		return
	}

	for i := range result.Items {
		result.Items[i] = *result.Items[i].Redacted()
	}
	writeJSON(w, http.StatusOK, result)
}

var varNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateVariables checks names can be referenced as ${NAME} in plans.
func validateVariables(w http.ResponseWriter, vars map[string]db.Variable) bool {
	for name := range vars {
		if !varNameRe.MatchString(name) {
			badRequest(w, "VALIDATION_ERROR", "Variable name "+strconv.Quote(name)+" must start with a letter or underscore and contain only letters, digits and underscores", map[string]string{"field": "variables"})
			return false
		}
	}
	return true
}

func (h *EnvironmentHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Environment name is required", map[string]string{"field": "name"})
		return
	}
	if !validateVariables(w, req.Variables) {
		return
	}

	env, err := h.store.Create(req.Name, req.Variables)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, env.Redacted())
}

func (h *EnvironmentHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	env, err := h.store.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if env == nil {
		notFound(w, "ENVIRONMENT", id)
		return
	}

	writeJSON(w, http.StatusOK, env.Redacted())
}

func (h *EnvironmentHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req updateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Environment name is required", map[string]string{"field": "name"})
		return
	}

	if !validateVariables(w, req.Variables) {
		return
	}

	env, err := h.store.Update(id, req.Name, req.Variables)
	if err != nil {
		internalError(w, err)
		return
	}
	if env == nil {
		notFound(w, "ENVIRONMENT", id)
		return
	}

	writeJSON(w, http.StatusOK, env.Redacted())
}

func (h *EnvironmentHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.store.Delete(id); err != nil {
		notFound(w, "ENVIRONMENT", id)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
