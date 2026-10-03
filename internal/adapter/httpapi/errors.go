package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type errorResponse struct {
	Error   string `json:"error"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Default().ErrorContext(nil, "write json response", "error", err)
	}
}
func writeError(w http.ResponseWriter, status int, code, field, message string) {
	writeJSON(w, status, errorResponse{Error: code, Field: field, Message: message})
}
