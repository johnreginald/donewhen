package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-187: link_commit is idempotent and issue_by_commit is strict.
func TestLinkCommitAndIssueByCommit(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "commits", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	_ = json.Unmarshal([]byte(out), &is)

	for i := 0; i < 2; i++ {
		if out, isErr = e.call(ctx, "link_commit", map[string]any{"issue": is.Key, "sha": "ABCDEF0123456", "message": "m"}); isErr {
			t.Fatal(out)
		}
	}
	cs, err := e.store.ListCommits(context.Background(), w.ID, is.ID)
	if err != nil || len(cs) != 1 {
		t.Fatalf("commits = %+v, %v", cs, err)
	}
	if out, isErr = e.call(ctx, "link_commit", map[string]any{"issue": is.Key, "sha": "nothex!"}); !isErr || !strings.Contains(out, "sha must be hex") {
		t.Fatalf("link_commit bad sha: err=%v %s", isErr, out)
	}

	for in, want := range map[string]string{"": "sha required", "a": "at least 7", "abc%def": "sha must be hex"} {
		out, isErr := e.call(ctx, "issue_by_commit", map[string]any{"sha": in})
		if !isErr || !strings.Contains(out, want) {
			t.Errorf("issue_by_commit %q: err=%v %s", in, isErr, out)
		}
	}
	// A 7-char prefix, in either case, finds the issue.
	for _, q := range []string{"abcdef0", "ABCDEF0"} {
		out, isErr := e.call(ctx, "issue_by_commit", map[string]any{"sha": q})
		if isErr || !strings.Contains(out, is.Key) {
			t.Errorf("issue_by_commit %q: err=%v %s", q, isErr, out)
		}
	}
	if out, isErr = e.call(ctx, "issue_by_commit", map[string]any{"sha": "1234567"}); !isErr || !strings.Contains(out, "not found") {
		t.Errorf("no match: err=%v %s", isErr, out)
	}
}
