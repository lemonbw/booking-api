package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const defaultMaxBodyBytes = 64 * 1024

const unknownFieldMarker = "unknown field "

func (h *Handler) decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBody)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		h.writeDecodeError(w, err)
		return false
	}
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "body", "request body is larger than the configured limit")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "body", "request body must hold a single json object")
		return false
	}
	return true
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "body", "request body is larger than the configured limit")
	case strings.Contains(err.Error(), unknownFieldMarker):
		writeError(w, http.StatusBadRequest, "invalid_json", "body", "request body holds an unknown field")
	default:
		writeError(w, http.StatusBadRequest, "invalid_json", "body", "request body must be valid JSON")
	}
}
