package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// APIError represents a structured error response per docs/04-data-model-and-api.md section 4.
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type errorResponse struct {
	Error APIError `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func notFound(w http.ResponseWriter, resource, id string) {
	writeError(w, http.StatusNotFound, resource+"_NOT_FOUND",
		resource+" with ID "+id+" does not exist", nil)
}

func badRequest(w http.ResponseWriter, code, message string, details interface{}) {
	writeError(w, http.StatusBadRequest, code, message, details)
}

// internalError logs the cause server-side and returns a generic message;
// details never reach the client (no stack traces in the UI).
func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
		"An internal error occurred. Check the control-plane log for details.", nil)
}
