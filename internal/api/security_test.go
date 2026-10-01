package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-195 part 1: both import routes are for an owner/admin browser session.
func TestImportIsAdminAndSessionOnly(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	wsp := e.workspace(owner)
	member, _ := e.user("")
	if err := e.store.AddMember(t.Context(), wsp.ID, member, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	ownerBrowser := browser(e.session(owner))
	memberBrowser := browser(e.session(member))
	ownerToken := bearer(e.token(owner, wsp.ID))

	payload := map[string]any{"issues": []map[string]any{{"id": "IMP-" + uniq(), "title": "imported"}}}
	routes := map[string]any{
		"/api/import":              payload,
		"/api/import/descriptions": map[string]string{"NOPE-1": "x"},
	}
	for path, body := range routes {
		if w := e.do("POST", path, body, memberBrowser); w.Code != 403 {
			t.Errorf("member %s = %d, want 403: %s", path, w.Code, w.Body.String())
		}
		if w := e.do("POST", path, body, ownerToken); w.Code != 403 {
			t.Errorf("bearer %s = %d, want 403: %s", path, w.Code, w.Body.String())
		}
		if w := e.do("POST", path, body, ownerBrowser); w.Code != 200 {
			t.Errorf("owner %s = %d, want 200: %s", path, w.Code, w.Body.String())
		}
	}
}

// PP-195 part 2: PATCH and DELETE take a human key, like GET.
func TestIssuePatchAndDeleteAcceptKeys(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	wsp := e.workspace(owner)
	ob := browser(e.session(owner))

	w := e.do("POST", "/api/issues", map[string]any{"title": "by key"}, ob)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var is models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &is)
	if is.WorkspaceID != wsp.ID {
		t.Fatalf("created in %s, want %s", is.WorkspaceID, wsp.ID)
	}

	w = e.do("PATCH", "/api/issues/"+is.Key, map[string]any{"title": "renamed"}, ob)
	if w.Code != 200 {
		t.Fatalf("PATCH by key = %d: %s", w.Code, w.Body.String())
	}
	var got models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.ID != is.ID || got.Title != "renamed" {
		t.Fatalf("patched %+v", got)
	}

	if w = e.do("PATCH", "/api/issues/NOPE-1", map[string]any{"title": "x"}, ob); w.Code != 404 {
		t.Errorf("PATCH unknown key = %d, want 404", w.Code)
	}
	if w = e.do("DELETE", "/api/issues/NOPE-1", nil, ob); w.Code != 404 {
		t.Errorf("DELETE unknown key = %d, want 404", w.Code)
	}
	if w = e.do("DELETE", "/api/issues/"+is.Key, nil, ob); w.Code != 200 {
		t.Fatalf("DELETE by key = %d: %s", w.Code, w.Body.String())
	}
	if w = e.do("GET", "/api/issues/"+is.ID, nil, ob); w.Code != 404 {
		t.Errorf("deleted issue GET = %d, want 404", w.Code)
	}

	// A key from another workspace is not found here, not leaked.
	other, _ := e.user("")
	e.workspace(other)
	ow := e.do("POST", "/api/issues", map[string]any{"title": "theirs"}, browser(e.session(other)))
	var theirs models.Issue
	_ = json.Unmarshal(ow.Body.Bytes(), &theirs)
	if w = e.do("DELETE", "/api/issues/"+theirs.Key, nil, ob); w.Code != 404 {
		t.Errorf("DELETE foreign key = %d, want 404", w.Code)
	}
}

// PP-195 part 3: an unexpected error is a generic body with a request id and
// no SQL text; the detail is in the log, not the response.
func TestInternalErrorBodyIsGeneric(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set(requestIDHeader, "req123")
	pgErr := &pgconn.PgError{Code: "42P01", Message: `relation "issues" does not exist`}
	if !handleStoreErr(rec, errors.Join(errors.New("select issues"), pgErr)) {
		t.Fatal("not handled")
	}
	if rec.Code != 500 {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "internal" || body["requestId"] != "req123" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "issues") {
		t.Fatalf("SQL text leaked: %s", rec.Body.String())
	}
}

// End to end: a real database error comes back generic, and the id on the body
// matches the response header.
func TestForcedDBErrorHasNoSQLText(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	ob := browser(e.session(owner))

	// 36 chars with dashes in the uuid places but not hex: it is taken for a
	// uuid, and Postgres rejects it with a type error.
	bad := "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"
	w := e.do("GET", "/api/issues/"+bad, nil, ob)
	if w.Code != 500 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "internal" || body["requestId"] == "" || body["requestId"] != w.Header().Get(requestIDHeader) {
		t.Fatalf("body = %s, header %q", w.Body.String(), w.Header().Get(requestIDHeader))
	}
	for _, leak := range []string{"uuid", "SQLSTATE", "invalid input", "issues"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("body leaks %q: %s", leak, w.Body.String())
		}
	}
}
