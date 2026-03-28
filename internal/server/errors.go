package server

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message, correlationID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorBody{
			Code:          code,
			Message:       message,
			CorrelationID: correlationID,
		},
	})
}

func BadRequest(w http.ResponseWriter, code, message, correlationID string) {
	writeError(w, http.StatusBadRequest, code, message, correlationID)
}

func Unauthorized(w http.ResponseWriter, correlationID string) {
	writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", correlationID)
}

func Forbidden(w http.ResponseWriter, correlationID string) {
	writeError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions", correlationID)
}

func NotFound(w http.ResponseWriter, code, message, correlationID string) {
	writeError(w, http.StatusNotFound, code, message, correlationID)
}

func BadGateway(w http.ResponseWriter, message, correlationID string) {
	writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", message, correlationID)
}

func ServiceUnavailable(w http.ResponseWriter, message, correlationID string) {
	writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", message, correlationID)
}

func InternalError(w http.ResponseWriter, correlationID string) {
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred", correlationID)
}
