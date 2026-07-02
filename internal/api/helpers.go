package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"raenil/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleStoreErr maps store errors to HTTP responses; returns true if handled.
func handleStoreErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return true
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
	return true
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20)) // 2 MiB
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// strPtr returns nil for empty string, else &s.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// orEmpty returns a non-nil slice so JSON encodes [] instead of null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
