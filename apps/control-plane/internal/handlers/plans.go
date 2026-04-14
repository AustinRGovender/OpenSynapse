package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type PlanHandlers struct {
	store *db.PlanStore
}

func NewPlanHandlers(store *db.PlanStore) *PlanHandlers {
	return &PlanHandlers{store: store}
}

type createPlanRequest struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Tags                 []string `json:"tags"`
	DefaultEnvironmentID *string  `json:"default_environment_id"`
	Root                 db.Node  `json:"root"`
}

type updatePlanRequest struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Tags                 []string `json:"tags"`
	DefaultEnvironmentID *string  `json:"default_environment_id"`
	Root                 db.Node  `json:"root"`
}

func (h *PlanHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cursor := r.URL.Query().Get("cursor")

	result, err := h.store.List(db.ListParams{Limit: limit, Cursor: cursor})
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *PlanHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Plan name is required", map[string]string{"field": "name"})
		return
	}

	plan, err := h.store.Create(req.Name, req.Description, req.Tags, req.Root, req.DefaultEnvironmentID)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

func (h *PlanHandlers) Get(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, plan)
}

func (h *PlanHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req updatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Plan name is required", map[string]string{"field": "name"})
		return
	}

	plan, err := h.store.Update(id, req.Name, req.Description, req.Tags, req.Root, req.DefaultEnvironmentID)
	if err != nil {
		internalError(w, err)
		return
	}
	if plan == nil {
		notFound(w, "PLAN", id)
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (h *PlanHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.store.Delete(id); err != nil {
		notFound(w, "PLAN", id)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PlanHandlers) ListVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Check plan exists
	plan, err := h.store.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if plan == nil {
		notFound(w, "PLAN", id)
		return
	}

	versions, err := h.store.ListVersions(id)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": versions})
}

func (h *PlanHandlers) GetVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	versionStr := r.PathValue("version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		badRequest(w, "INVALID_VERSION", "Version must be an integer", nil)
		return
	}

	v, err := h.store.GetVersion(id, version)
	if err != nil {
		internalError(w, err)
		return
	}
	if v == nil {
		notFound(w, "PLAN_VERSION", versionStr)
		return
	}

	writeJSON(w, http.StatusOK, v)
}

func (h *PlanHandlers) Validate(w http.ResponseWriter, r *http.Request) {
	var req createPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	// Basic validation for now; full schema validation will use JSON schemas
	errors := validatePlanBasic(req.Root)
	if len(errors) > 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":  false,
			"errors": errors,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":  true,
		"errors": []string{},
	})
}

func validatePlanBasic(root db.Node) []string {
	var errs []string
	if root.Type == "" {
		errs = append(errs, "root node must have a type")
	}
	return errs
}
