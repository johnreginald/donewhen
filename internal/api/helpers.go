package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"raenil/internal/auth"
	"raenil/internal/models"
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
	if errors.Is(err, store.ErrNotMember) {
		writeErr(w, http.StatusForbidden, "not a member of this workspace")
		return true
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return true
	}
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, err.Error())
		return true
	}
	if errors.Is(err, store.ErrInvalid) {
		writeErr(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "invalid: "))
		return true
	}
	internalErr(w, err)
	return true
}

// internalErr answers 500 with a plain message. Driver and SQL detail can leak
// schema, so it goes to the log, never to the client.
func internalErr(w http.ResponseWriter, err error) {
	log.Printf("api: internal error: %v", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

// ws returns the workspace id this request acts on. Handlers behind wsGuard can
// rely on it being present; it is passed explicitly into every store call so a
// missing scope is a compile error rather than a leak.
func ws(r *http.Request) string {
	w, _ := auth.WorkspaceFrom(r.Context())
	return w.ID
}

// canAdmin reports whether the caller may administer the active workspace.
func canAdmin(r *http.Request) bool {
	return models.CanAdmin(auth.RoleFrom(r.Context()))
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

// errInvalid is a bad-request error built in the API layer.
func errInvalid(msg string) error {
	return fmt.Errorf("%w: %s", store.ErrInvalid, msg)
}
