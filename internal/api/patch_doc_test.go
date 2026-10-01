package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-182: REST PATCH of a document changes only the fields sent; a missing
// issueId keeps the link, null or "" detaches it.
func TestPatchDocumentMergePatch(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	opt := browser(e.session(owner))

	w := e.do("POST", "/api/issues", map[string]any{"title": "doc issue", "stateName": "Backlog"}, opt)
	var is models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &is)
	w = e.do("POST", "/api/documents", map[string]any{
		"title": "T", "bodyMd": "body", "type": "decision", "issueId": is.ID,
	}, opt)
	if w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var doc models.Document
	_ = json.Unmarshal(w.Body.Bytes(), &doc)

	patch := func(body map[string]any) (int, models.Document) {
		t.Helper()
		w := e.do("PATCH", "/api/documents/"+doc.ID, body, opt)
		var got models.Document
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		return w.Code, got
	}
	code, got := patch(map[string]any{"bodyMd": "new"})
	if code != 200 || got.BodyMD != "new" || got.Title != "T" || got.Type != "decision" ||
		got.IssueID == nil || *got.IssueID != is.ID {
		t.Fatalf("body-only: %d %+v", code, got)
	}
	// A bad label id: clean 400, nothing saved.
	w = e.do("PATCH", "/api/documents/"+doc.ID,
		map[string]any{"bodyMd": "label fail", "labelIds": []string{"00000000-0000-0000-0000-000000000000"}}, opt)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad label: %d %s", w.Code, w.Body.String())
	}
	if code, got = patch(map[string]any{}); code != 200 || got.BodyMD != "new" {
		t.Fatalf("label error left a partial save: %d %+v", code, got)
	}
	// Unknown issue: 404, nothing saved.
	w = e.do("PATCH", "/api/documents/"+doc.ID,
		map[string]any{"title": "changed", "issueId": "00000000-0000-0000-0000-000000000000"}, opt)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown issue: %d", w.Code)
	}
	if _, got = patch(map[string]any{}); got.Title != "T" {
		t.Fatalf("title changed: %+v", got)
	}
	// null detaches.
	if code, got = patch(map[string]any{"issueId": nil}); code != 200 || got.IssueID != nil || got.Title != "T" {
		t.Fatalf("detach: %d %+v", code, got)
	}
}
