package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type FragmentHandlers struct {
	fragments *db.FragmentStore
}

func NewFragmentHandlers(fragments *db.FragmentStore) *FragmentHandlers {
	return &FragmentHandlers{fragments: fragments}
}

func (h *FragmentHandlers) List(w http.ResponseWriter, r *http.Request) {
	result, err := h.fragments.List()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FragmentHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Tags        []string     `json:"tags"`
		NodeSubtree db.Node      `json:"node_subtree"`
		Bindings    []db.Binding `json:"bindings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if req.Name == "" {
		badRequest(w, "VALIDATION_ERROR", "Fragment name is required", nil)
		return
	}

	frag, err := h.fragments.Create(req.Name, req.Description, req.Tags, req.NodeSubtree, req.Bindings, false)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, frag)
}

func (h *FragmentHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	frag, err := h.fragments.Get(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if frag == nil {
		notFound(w, "FRAGMENT", id)
		return
	}
	writeJSON(w, http.StatusOK, frag)
}

func (h *FragmentHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Tags        []string     `json:"tags"`
		NodeSubtree db.Node      `json:"node_subtree"`
		Bindings    []db.Binding `json:"bindings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	frag, err := h.fragments.Update(id, req.Name, req.Description, req.Tags, req.NodeSubtree, req.Bindings)
	if err != nil {
		writeError(w, http.StatusForbidden, "CANNOT_MODIFY", err.Error(), nil)
		return
	}
	if frag == nil {
		notFound(w, "FRAGMENT", id)
		return
	}
	writeJSON(w, http.StatusOK, frag)
}

func (h *FragmentHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.fragments.Delete(id); err != nil {
		if err.Error() == "cannot delete built-in fragment" {
			writeError(w, http.StatusForbidden, "CANNOT_DELETE_BUILTIN", err.Error(), nil)
		} else {
			notFound(w, "FRAGMENT", id)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
