package handlers

import (
	"encoding/json"
	"net/http"
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
	Name      string              `json:"name"`
	Variables map[string]db.Variable `json:"variables"`
}

type updateEnvironmentRequest struct {
	Name      string              `json:"name"`
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

	writeJSON(w, http.StatusOK, result)
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

	env, err := h.store.Create(req.Name, req.Variables)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, env)
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

	// Redact secret variable values
	redacted := redactSecrets(env)
	writeJSON(w, http.StatusOK, redacted)
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

	env, err := h.store.Update(id, req.Name, req.Variables)
	if err != nil {
		internalError(w, err)
		return
	}
	if env == nil {
		notFound(w, "ENVIRONMENT", id)
		return
	}

	writeJSON(w, http.StatusOK, redactSecrets(env))
}

func (h *EnvironmentHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.store.Delete(id); err != nil {
		notFound(w, "ENVIRONMENT", id)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// redactSecrets returns a copy of the environment with secret values replaced.
func redactSecrets(env *db.Environment) *db.Environment {
	redacted := *env
	redacted.Variables = make(map[string]db.Variable, len(env.Variables))
	for k, v := range env.Variables {
		if v.Secret {
			redacted.Variables[k] = db.Variable{Value: "***REDACTED***", Secret: true}
		} else {
			redacted.Variables[k] = v
		}
	}
	return &redacted
}
