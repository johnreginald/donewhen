package mcp

import (
	"context"
	"strings"
	"testing"
)

// PP-188 part 2: a pinned token is re-checked against membership on every
// path, including the write scope that used to return the pin unchecked.
func TestPinnedTokenLosesAccessWhenMembershipGoes(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	a := e.workspace(uid)
	ctx := e.ctxFor(uid, a.ID)

	// While a member, the pinned token works for reads and writes.
	if out, isErr := e.call(ctx, "list_issues", nil); isErr {
		t.Fatalf("member list_issues: %s", out)
	}
	if out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "ok"}); isErr {
		t.Fatalf("member save_issue: %s", out)
	}

	// Remove the membership directly so the token row survives and only the
	// per-call re-check stands between the token and the workspace.
	if _, err := e.pool.Exec(context.Background(),
		`DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, a.ID, uid); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		tool string
		args map[string]any
		want string // substring of the error; "" means any error
	}{
		{"list_issues (read scope)", "list_issues", nil, ""},
		{"save_issue create (write scope, pin branch)", "save_issue", map[string]any{"title": "nope"}, "workspace access revoked"},
		{"list_projects", "list_projects", nil, ""},
		{"save_issue naming the pinned workspace", "save_issue", map[string]any{"title": "nope", "workspace": a.ID}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, isErr := e.call(ctx, tc.tool, tc.args)
			if !isErr {
				t.Fatalf("removed member's pinned token succeeded: %s", out)
			}
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Fatalf("error = %q, want it to contain %q", out, tc.want)
			}
		})
	}

	// Nothing was written by the refused calls.
	var n int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM issues WHERE workspace_id=$1 AND title='nope'`, a.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused call wrote %d issues (err %v)", n, err)
	}
}

// A pinned token keeps working inside its own workspace and cannot name another.
func TestPinnedTokenStaysInItsWorkspace(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	a := e.workspace(uid)
	b := e.workspace(uid)
	ctx := e.ctxFor(uid, a.ID)

	if out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "in a"}); isErr {
		t.Fatalf("pinned write in own workspace: %s", out)
	}
	if out, isErr := e.call(ctx, "list_issues", map[string]any{"workspace": b.ID}); !isErr {
		t.Fatalf("pinned token read another workspace: %s", out)
	}
	if out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "in b", "workspace": b.ID}); !isErr {
		t.Fatalf("pinned token wrote another workspace: %s", out)
	}
}
