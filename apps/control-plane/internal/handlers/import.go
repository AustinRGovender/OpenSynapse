package handlers

import (
	"net/http"
	"strings"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/importer"
)

type ImportHandlers struct {
	plans *db.PlanStore
}

func NewImportHandlers(plans *db.PlanStore) *ImportHandlers {
	return &ImportHandlers{plans: plans}
}

// ImportJMX handles POST /api/v1/plans/import/jmx
func (h *ImportHandlers) ImportJMX(w http.ResponseWriter, r *http.Request) {
	// Accept both multipart file upload and raw XML body
	var result *importer.ImportResult
	var err error

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart") {
		// Multipart file upload
		file, _, fileErr := r.FormFile("file")
		if fileErr != nil {
			badRequest(w, "NO_FILE", "No .jmx file provided. Upload as 'file' field.", nil)
			return
		}
		defer file.Close()
		result, err = importer.ImportJMX(file)
	} else {
		// Raw XML body
		result, err = importer.ImportJMX(r.Body)
	}

	if err != nil {
		badRequest(w, "IMPORT_FAILED", err.Error(), nil)
		return
	}

	// Save the plan
	plan := result.Plan
	saved, saveErr := h.plans.Create(plan.Name, plan.Description, plan.Tags, plan.Root, nil)
	if saveErr != nil {
		internalError(w, saveErr)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"plan": saved,
		"log":  result.Log,
	})
}
