package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"raenil/internal/store"
)

func TestRESTLinkURLsAreHTTPOnly(t *testing.T) {
	st := testStore(t)
	u := newUser(t, st)
	w := newWorkspace(t, st, u)
	srv := &Server{store: st}
	is, err := st.CreateIssue(t.Context(), w.ID, store.IssueInput{Title: "x", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	path := map[string]string{"id": is.Key}

	bad := []string{"javascript:alert(1)", "data:text/html,x", "vbscript:x", "file:///etc/passwd"}
	for _, b := range bad {
		rec := call(t, srv.handleSetDev, u, w, "PUT", `{"prUrl":"`+b+`"}`, path)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_url") {
			t.Errorf("set dev %q: %d %s", b, rec.Code, rec.Body)
		}
		rec = call(t, srv.handleAddCommit, u, w, "POST", `{"sha":"abc1234","message":"m","url":"`+b+`"}`, path)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_url") {
			t.Errorf("add commit %q: %d %s", b, rec.Code, rec.Body)
		}
		rec = call(t, srv.handleSaveProject, u, w, "POST", `{"name":"p","repoUrl":"`+b+`"}`, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_url") {
			t.Errorf("save project %q: %d %s", b, rec.Code, rec.Body)
		}
		rec = call(t, srv.handleSaveInitiative, u, w, "POST", `{"name":"i","repoUrl":"`+b+`"}`, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_url") {
			t.Errorf("save initiative %q: %d %s", b, rec.Code, rec.Body)
		}
	}

	rec := call(t, srv.handleSetDev, u, w, "PUT", `{"prUrl":"https://github.com/x/y/pull/1"}`, path)
	if rec.Code != http.StatusOK {
		t.Errorf("valid PR url: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, srv.handleAddCommit, u, w, "POST", `{"sha":"abc1234","message":"m","url":"https://github.com/x/y/commit/abc1234"}`, path)
	if rec.Code != http.StatusCreated {
		t.Errorf("valid commit url: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, srv.handleSetDev, u, w, "PUT", `{"prUrl":""}`, path)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "github.com") {
		t.Errorf("empty prUrl should clear: %d %s", rec.Code, rec.Body)
	}
}

func TestInternalErrorsAreGeneric(t *testing.T) {
	rec := httptest.NewRecorder()
	handleStoreErr(rec, errSecret{})
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("500 leaked detail: %d %s", rec.Code, rec.Body)
	}
}

type errSecret struct{}

func (errSecret) Error() string { return "pq: secret connection detail" }
