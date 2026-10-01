package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-183: set_issue_dev changes only the fields sent; neither is an error.
func TestSetIssueDevMCP(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)
	bg := context.Background()

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "dev", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	_ = json.Unmarshal([]byte(out), &is)
	get := func() models.Issue {
		got, err := e.store.GetIssue(bg, w.ID, is.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	set := func(args map[string]any) (string, bool) {
		args["issue"] = is.Key
		return e.call(ctx, "set_issue_dev", args)
	}

	if out, isErr = set(map[string]any{"gitBranch": "feat/x"}); isErr {
		t.Fatal(out)
	}
	if out, isErr = set(map[string]any{"prUrl": "https://github.com/x/y/pull/1"}); isErr {
		t.Fatal(out)
	}
	if got := get(); got.GitBranch == nil || *got.GitBranch != "feat/x" || got.PRURL == nil {
		t.Fatalf("prUrl wiped the branch: %+v", got)
	}
	if out, isErr = set(map[string]any{"gitBranch": "feat/y"}); isErr {
		t.Fatal(out)
	}
	if got := get(); got.GitBranch == nil || *got.GitBranch != "feat/y" || got.PRURL == nil {
		t.Fatalf("branch wiped the PR: %+v", got)
	}
	if out, isErr = set(map[string]any{"gitBranch": ""}); isErr {
		t.Fatal(out)
	}
	if got := get(); got.GitBranch != nil || got.PRURL == nil {
		t.Fatalf("empty branch: %+v", got)
	}
	out, isErr = set(map[string]any{})
	if !isErr || !strings.Contains(out, "nothing to set") {
		t.Fatalf("neither: err=%v %s", isErr, out)
	}
	if got := get(); got.PRURL == nil {
		t.Fatalf("the failed call changed the issue: %+v", got)
	}
}
