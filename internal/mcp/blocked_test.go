package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-207: save_issue needs a reason to move an issue to Blocked.
func TestSaveIssueBlockedNeedsReason(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "stuck", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	if err := json.Unmarshal([]byte(out), &is); err != nil {
		t.Fatal(err)
	}

	out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": "Blocked"})
	if !isErr || !strings.Contains(out, "reason") {
		t.Fatalf("no reason: err=%v %s", isErr, out)
	}
	if out, isErr = e.call(ctx, "save_issue", map[string]any{"id": is.Key, "state": "Blocked", "reason": "needs a decision"}); isErr {
		t.Fatalf("with reason: %s", out)
	}
	cs, err := e.store.ListComments(context.Background(), w.ID, is.ID)
	if err != nil || len(cs) != 1 || cs[0].Kind != "blocked_reason" || cs[0].Actor != "ai" {
		t.Fatalf("comments = %+v err=%v", cs, err)
	}
}
