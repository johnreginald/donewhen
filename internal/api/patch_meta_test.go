package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-181: REST PATCH of an epic or initiative changes only the fields sent;
// an explicit null clears repoUrl / detaches the initiative.
func TestPatchProjectAndInitiativeMergePatch(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	opt := browser(e.session(owner))

	var ini models.Initiative
	w := e.do("POST", "/api/initiatives", map[string]any{"name": "ini", "descriptionMd": "ini desc"}, opt)
	if w.Code != 200 {
		t.Fatalf("create ini: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ini)
	var p models.Project
	w = e.do("POST", "/api/projects", map[string]any{
		"name": "epic", "descriptionMd": "desc", "initiativeId": ini.ID, "repoUrl": "https://github.com/x/y",
	}, opt)
	if w.Code != 200 {
		t.Fatalf("create epic: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.Status != "active" {
		t.Fatalf("defaults not applied: %+v", p)
	}
	patch := func(path string, body map[string]any, into any) {
		t.Helper()
		w := e.do("PATCH", path, body, opt)
		if w.Code != http.StatusOK {
			t.Fatalf("PATCH %s: %d %s", path, w.Code, w.Body.String())
		}
		_ = json.Unmarshal(w.Body.Bytes(), into)
	}

	var got models.Project
	patch("/api/projects/"+p.ID, map[string]any{"name": "renamed"}, &got)
	if got.Name != "renamed" || got.DescriptionMD != "desc" || got.InitiativeID == nil || got.RepoURL == nil {
		t.Fatalf("name-only patch: %+v", got)
	}
	// What the web composer sends to clear an epic's repo and initiative.
	patch("/api/projects/"+p.ID, map[string]any{"initiativeId": nil, "repoUrl": nil}, &got)
	if got.InitiativeID != nil || got.RepoURL != nil || got.Name != "renamed" || got.DescriptionMD != "desc" {
		t.Fatalf("null patch: %+v", got)
	}

	var gi models.Initiative
	patch("/api/initiatives/"+ini.ID, map[string]any{"name": "ini2"}, &gi)
	if gi.Name != "ini2" || gi.DescriptionMD != "ini desc" {
		t.Fatalf("initiative patch: %+v", gi)
	}

	if w = e.do("PATCH", "/api/projects/00000000-0000-0000-0000-000000000000", map[string]any{"name": "x"}, opt); w.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", w.Code)
	}
	if w = e.do("PATCH", "/api/projects/"+p.ID, map[string]any{"name": ""}, opt); w.Code != http.StatusBadRequest {
		t.Fatalf("empty name: %d", w.Code)
	}
}
