package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
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
	var ge *store.GateError
	if errors.As(err, &ge) {
		writeGateErr(w, ge)
		return true
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

// writeGateErr answers 409 with the stable code and the open criteria, so a
// client can show them without parsing the message.
func writeGateErr(w http.ResponseWriter, ge *store.GateError) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": ge.Error(),
		"code":  ge.Code,
		"state": ge.State,
		"open":  orEmpty(ge.Open),
	})
}

// internalErr answers 500 with a generic body and a request id. Driver and SQL
// detail can leak schema, so the full error goes to the log under that id,
// never to the client. The id comes from the requestID middleware.
func internalErr(w http.ResponseWriter, err error) {
	id := w.Header().Get(requestIDHeader)
	log.Printf("api: internal error (request %s): %v", id, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal", "requestId": id})
}

// requestIDHeader carries the per-request id on the response so a client can
// quote it and an operator can find the matching log line.
const requestIDHeader = "X-Request-Id"

// requestID stamps every response with a fresh random id. An inbound value is
// ignored: it would let a caller forge log correlation.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := auth.RandomToken(8)
		if err != nil {
			raw = "unknown"
		}
		w.Header().Set(requestIDHeader, raw)
		next.ServeHTTP(w, r)
	})
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
