package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-185: PATCH /api/issues/{id} refuses a save whose expectedUpdatedAt is
// stale, leaves the newer edit in place, and still works without the field.
func TestIssueUpdateStaleGuard(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	wsp := e.workspace(owner)
	opt := browser(e.session(owner))
	tok := bearer(e.token(owner, wsp.ID))

	w := e.do("POST", "/api/issues", map[string]any{"title": "v1", "stateName": "Backlog"}, opt)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var loaded models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &loaded)

	// Current expectedUpdatedAt: applied.
	w = e.do("PATCH", "/api/issues/"+loaded.ID,
		map[string]any{"title": "v2", "expectedUpdatedAt": loaded.UpdatedAt}, opt)
	if w.Code != 200 {
		t.Fatalf("current: %d %s", w.Code, w.Body.String())
	}
	var v2 models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &v2)
	if v2.Title != "v2" || !v2.UpdatedAt.After(loaded.UpdatedAt) {
		t.Fatalf("v2 = %+v", v2)
	}

	// Someone else edits without the field (MCP style): still works.
	w = e.do("PATCH", "/api/issues/"+loaded.ID, map[string]any{"title": "from mcp"}, tok)
	if w.Code != 200 {
		t.Fatalf("no expectedUpdatedAt: %d %s", w.Code, w.Body.String())
	}

	// The first page now saves from a stale copy: 409, current issue in the
	// body, the newer edit untouched.
	w = e.do("PATCH", "/api/issues/"+loaded.ID,
		map[string]any{"title": "stale save", "expectedUpdatedAt": v2.UpdatedAt}, opt)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Code  string       `json:"code"`
		Issue models.Issue `json:"issue"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Code != "stale" || body.Issue.Title != "from mcp" {
		t.Fatalf("body = %s", w.Body.String())
	}
	stored, err := e.store.GetIssue(t.Context(), wsp.ID, loaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "from mcp" || !stored.UpdatedAt.Equal(body.Issue.UpdatedAt) {
		t.Fatalf("stale save changed the issue: %+v", stored)
	}

	// Retrying with the current value succeeds.
	w = e.do("PATCH", "/api/issues/"+loaded.ID,
		map[string]any{"title": "merged", "expectedUpdatedAt": body.Issue.UpdatedAt}, opt)
	if w.Code != 200 {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	var merged models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &merged)

	// A label-only change is an edit too: it moves updatedAt, so a page that
	// loaded before it cannot overwrite it blindly.
	w = e.do("PATCH", "/api/issues/"+loaded.ID, map[string]any{"labelNames": []string{"bug"}}, tok)
	if w.Code != 200 {
		t.Fatalf("labels: %d %s", w.Code, w.Body.String())
	}
	w = e.do("PATCH", "/api/issues/"+loaded.ID,
		map[string]any{"title": "late", "expectedUpdatedAt": merged.UpdatedAt}, opt)
	if w.Code != http.StatusConflict {
		t.Fatalf("after label change: %d %s", w.Code, w.Body.String())
	}
}
