package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-183: PATCH /api/issues/{id}/dev changes only the fields present.
func TestPatchIssueDev(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	opt := browser(e.session(owner))

	w := e.do("POST", "/api/issues", map[string]any{"title": "dev", "stateName": "Backlog"}, opt)
	var is models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &is)
	patch := func(body map[string]any) (int, models.Issue) {
		w := e.do("PATCH", "/api/issues/"+is.ID+"/dev", body, opt)
		var got models.Issue
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		return w.Code, got
	}
	if code, _ := patch(map[string]any{"gitBranch": "feat/x"}); code != 200 {
		t.Fatalf("branch: %d", code)
	}
	code, got := patch(map[string]any{"prUrl": "https://github.com/x/y/pull/1"})
	if code != 200 || got.GitBranch == nil || *got.GitBranch != "feat/x" || got.PRURL == nil {
		t.Fatalf("prUrl wiped the branch: %d %+v", code, got)
	}
	code, got = patch(map[string]any{"gitBranch": ""})
	if code != 200 || got.GitBranch != nil || got.PRURL == nil {
		t.Fatalf("empty branch: %d %+v", code, got)
	}
	if code, _ = patch(map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("neither: %d", code)
	}
}
