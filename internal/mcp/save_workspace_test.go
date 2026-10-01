package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-189: save_workspace follows the REST rules.
func TestSaveWorkspaceRules(t *testing.T) {
	e := newEnv(t)
	owner := e.user()
	admin := e.user()
	member := e.user()
	a := e.workspace(owner)
	b := e.workspace(owner)
	for _, w := range []models.Workspace{a, b} {
		e.member(w.ID, admin, models.RoleAdmin)
		e.member(w.ID, member, models.RoleMember)
	}
	other := e.workspace(owner) // for the duplicate cases

	rename := func(ws string) map[string]any { return map[string]any{"id": ws, "name": "Renamed " + uniq()} }

	cases := []struct {
		name    string
		ctx     context.Context
		args    map[string]any
		wantErr string // "" = must succeed
		target  string // workspace whose name must (not) change
	}{
		{"owner unpinned renames own workspace", e.ctxFor(owner, ""), rename(a.ID), "", a.ID},
		{"admin unpinned renames own workspace", e.ctxFor(admin, ""), rename(a.ID), "", a.ID},
		{"member is refused", e.ctxFor(member, ""), rename(a.ID), "admin role required", a.ID},
		{"member pinned to the same workspace is refused", e.ctxFor(member, a.ID), rename(a.ID), "admin role required", a.ID},
		{"admin pinned to A cannot update B", e.ctxFor(admin, a.ID), rename(b.ID), "token is pinned to another workspace", b.ID},
		{"admin pinned to A updates A", e.ctxFor(admin, a.ID), rename(a.ID), "", a.ID},
		{"pinned token cannot reach B by slug either", e.ctxFor(admin, a.ID), map[string]any{"id": b.Slug, "name": "x"}, "token is pinned to another workspace", b.ID},
		{"non-member is refused", e.ctxFor(e.user(), ""), rename(a.ID), "not a member", a.ID},
		{"unknown workspace", e.ctxFor(owner, ""), rename("00000000-0000-0000-0000-000000000000"), "not found", ""},
		{"bearer cannot create a workspace", e.ctxFor(owner, ""),
			map[string]any{"name": "N " + uniq(), "keyPrefix": "Y" + uniq()}, "browser session", ""},
		{"pinned bearer cannot create a workspace", e.ctxFor(owner, a.ID),
			map[string]any{"name": "N " + uniq(), "keyPrefix": "Y" + uniq()}, "browser session", ""},
		{"empty name rejected", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "name": ""}, "name must not be empty", a.ID},
		{"blank name rejected", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "name": "   "}, "name must not be empty", a.ID},
		{"empty slug rejected", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "slug": ""}, "slug must not be empty", a.ID},
		{"slug that slugifies to nothing rejected", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "slug": "!!!"}, "slug must not be empty", a.ID},
		{"empty prefix rejected", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "keyPrefix": ""}, "keyPrefix must not be empty", a.ID},
		{"malformed prefix goes through ValidatePrefix", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "keyPrefix": "1ab"}, "starting with a letter", a.ID},
		{"reserved prefix refused", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "keyPrefix": "k"}, "reserved", a.ID},
		{"duplicate slug is a conflict", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "slug": other.Slug}, "already exists", a.ID},
		{"duplicate prefix is a conflict", e.ctxFor(owner, ""), map[string]any{"id": a.ID, "keyPrefix": other.KeyPrefix}, "already exists", a.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var before string
			if tc.target != "" {
				before = e.wsName(tc.target)
			}
			out, isErr := e.call(tc.ctx, "save_workspace", tc.args)
			if tc.wantErr == "" {
				if isErr {
					t.Fatalf("want success, got error: %s", out)
				}
				if want, _ := tc.args["name"].(string); want != "" && e.wsName(tc.target) != want {
					t.Fatalf("name = %q, want %q", e.wsName(tc.target), want)
				}
				return
			}
			if !isErr {
				t.Fatalf("want error containing %q, got success: %s", tc.wantErr, out)
			}
			if !strings.Contains(out, tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", out, tc.wantErr)
			}
			if tc.target != "" && e.wsName(tc.target) != before {
				t.Fatalf("refused call changed the workspace name %q -> %q", before, e.wsName(tc.target))
			}
		})
	}

	// Nothing the refused create calls tried to make exists.
	var n int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM workspaces WHERE name LIKE 'N %'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("refused create wrote %d workspaces (err %v)", n, err)
	}
}
