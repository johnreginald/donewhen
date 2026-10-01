package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/service"
	"github.com/johnreginald/donewhen/internal/store"
)

// PP-201: the MCP write scope. scopeOne is called directly (its contract) and
// through tools (what a client sees).

func (e *testEnv) deps() *deps {
	return &deps{svc: service.New(e.store, events.NewBus()), store: e.store, cfg: config.Config{}, mgr: e.mgr}
}

func scopeReq(args map[string]any) mcp.CallToolRequest {
	var r mcp.CallToolRequest
	r.Params.Arguments = args
	return r
}

func (e *testEnv) issue(wsID, title string) string {
	e.t.Helper()
	is, err := e.store.CreateIssue(context.Background(), wsID, store.IssueInput{Title: title})
	if err != nil {
		e.t.Fatal(err)
	}
	return is.Key
}

func TestScopeOneWithPin(t *testing.T) {
	e := newEnv(t)
	d := e.deps()
	uid := e.user()
	a := e.workspace(uid)
	b := e.workspace(uid)
	keyB := e.issue(b.ID, "in b")
	pinned := e.ctxFor(uid, a.ID)

	// No reference: the pin is the answer, not "your two workspaces, say which".
	if got, err := d.scopeOne(pinned, scopeReq(nil)); err != nil || got != a.ID {
		t.Fatalf("pinned, nothing named = %q %v, want %q", got, err, a.ID)
	}
	// A reference into another workspace the account belongs to must not steer a
	// pinned token there: the pin narrows, so the call stays in A (and then does
	// not find the issue).
	if got, err := d.scopeOne(pinned, scopeReq(nil), issueRef(keyB)); err != nil || got != a.ID {
		t.Fatalf("pinned, key from B = %q %v, want the pin %q", got, err, a.ID)
	}
	// Without a pin the same reference derives B.
	unpinned := e.ctxFor(uid, "")
	if got, err := d.scopeOne(unpinned, scopeReq(nil), issueRef(keyB)); err != nil || got != b.ID {
		t.Fatalf("unpinned, key from B = %q %v, want %q", got, err, b.ID)
	}
}

func TestScopeOneNamedVersusPin(t *testing.T) {
	e := newEnv(t)
	d := e.deps()
	uid := e.user()
	a := e.workspace(uid)
	b := e.workspace(uid)
	pinned := e.ctxFor(uid, a.ID)

	if got, err := d.scopeOne(pinned, scopeReq(map[string]any{"workspace": a.Slug})); err != nil || got != a.ID {
		t.Fatalf("named = the pin: %q %v", got, err)
	}
	for _, ref := range []string{b.ID, b.Slug, b.KeyPrefix} {
		if got, err := d.scopeOne(pinned, scopeReq(map[string]any{"workspace": ref})); !errors.Is(err, store.ErrNotMember) || got != "" {
			t.Fatalf("named %q conflicting with the pin = %q %v, want ErrNotMember", ref, got, err)
		}
	}
	// Named wins over a reference, for an unpinned token.
	unpinned := e.ctxFor(uid, "")
	keyB := e.issue(b.ID, "in b")
	if got, err := d.scopeOne(unpinned, scopeReq(map[string]any{"workspace": a.ID}), issueRef(keyB)); err != nil || got != a.ID {
		t.Fatalf("named beats key = %q %v, want %q", got, err, a.ID)
	}
}

func TestScopeOneInfersWorkspaceFromKey(t *testing.T) {
	e := newEnv(t)
	d := e.deps()
	uid := e.user()
	stranger := e.user()
	a := e.workspace(uid)
	b := e.workspace(uid)
	foreign := e.workspace(stranger)
	keyA := e.issue(a.ID, "in a")
	keyB := e.issue(b.ID, "in b")
	keyForeign := e.issue(foreign.ID, "not mine")
	ctx := e.ctxFor(uid, "")

	if got, err := d.scopeOne(ctx, scopeReq(nil), issueRef(keyA)); err != nil || got != a.ID {
		t.Fatalf("key A = %q %v", got, err)
	}
	if got, err := d.scopeOne(ctx, scopeReq(nil), issueRef(keyB)); err != nil || got != b.ID {
		t.Fatalf("key B = %q %v", got, err)
	}
	// The first reference that resolves decides.
	if got, err := d.scopeOne(ctx, scopeReq(nil), issueRef("NOPE-9"), issueRef(keyB)); err != nil || got != b.ID {
		t.Fatalf("unknown then key B = %q %v", got, err)
	}
	// Someone else's key never derives their workspace.
	if got, err := d.scopeOne(ctx, scopeReq(nil), issueRef(keyForeign)); !errors.Is(err, store.ErrNotMember) || got != "" {
		t.Fatalf("foreign key = %q %v, want ErrNotMember", got, err)
	}
	// Nothing to infer from, two workspaces → say which.
	if _, err := d.scopeOne(ctx, scopeReq(nil)); !errors.Is(err, auth.ErrAmbiguousWorkspace) {
		t.Fatalf("no reference, two workspaces = %v, want ErrAmbiguousWorkspace", err)
	}
	// A sole workspace is unambiguous.
	solo := e.user()
	s := e.workspace(solo)
	if got, err := d.scopeOne(e.ctxFor(solo, ""), scopeReq(nil)); err != nil || got != s.ID {
		t.Fatalf("sole membership = %q %v", got, err)
	}
	// And a user with none gets ErrNoWorkspace.
	if _, err := d.scopeOne(e.ctxFor(e.user(), ""), scopeReq(nil)); !errors.Is(err, auth.ErrNoWorkspace) {
		t.Fatalf("no membership = %v", err)
	}
}

// Through the tools: a pinned token cannot read or write an issue of another
// workspace of the same account by its key, and nothing is changed.
func TestPinnedToolCallsCannotReachOtherWorkspaceByKey(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	a := e.workspace(uid)
	b := e.workspace(uid)
	keyB := e.issue(b.ID, "in b")
	pinned := e.ctxFor(uid, a.ID)

	if out, isErr := e.call(pinned, "get_issue", map[string]any{"id": keyB}); !isErr {
		t.Fatalf("pinned get_issue of B's key succeeded: %s", out)
	}
	if out, isErr := e.call(pinned, "save_issue", map[string]any{"id": keyB, "title": "hacked"}); !isErr {
		t.Fatalf("pinned save_issue of B's key succeeded: %s", out)
	}
	if out, isErr := e.call(pinned, "delete_issue", map[string]any{"id": keyB}); !isErr {
		t.Fatalf("pinned delete_issue of B's key succeeded: %s", out)
	}
	is, err := e.store.GetIssueByKey(context.Background(), b.ID, keyB)
	if err != nil || is.Title != "in b" {
		t.Fatalf("B's issue was touched: %+v %v", is, err)
	}
	// The same calls work for an unpinned token, which derives B from the key.
	unpinned := e.ctxFor(uid, "")
	if out, isErr := e.call(unpinned, "get_issue", map[string]any{"id": keyB}); isErr || !strings.Contains(out, "in b") {
		t.Fatalf("unpinned get_issue by key: %s (err %v)", out, isErr)
	}
	if out, isErr := e.call(unpinned, "save_issue", map[string]any{"id": keyB, "title": "renamed"}); isErr {
		t.Fatalf("unpinned save_issue by key: %s", out)
	}
	if is, _ := e.store.GetIssueByKey(context.Background(), b.ID, keyB); is.Title != "renamed" {
		t.Fatalf("title = %q", is.Title)
	}
}

// The request context built by the auth middleware reaches the tool handlers:
// the account behind the token decides what a call sees and who is recorded.
func TestRequestContextReachesToolHandlers(t *testing.T) {
	e := newEnv(t)
	alice := e.user()
	bob := e.user()
	a := e.workspace(alice)
	b := e.workspace(bob)
	e.issue(a.ID, "alice only")
	e.issue(b.ID, "bob only")

	out, isErr := e.call(e.ctxFor(alice, ""), "list_issues", nil)
	if isErr || !strings.Contains(out, "alice only") || strings.Contains(out, "bob only") {
		t.Fatalf("alice list_issues = %s (err %v)", out, isErr)
	}
	out, isErr = e.call(e.ctxFor(bob, ""), "list_issues", nil)
	if isErr || !strings.Contains(out, "bob only") || strings.Contains(out, "alice only") {
		t.Fatalf("bob list_issues = %s (err %v)", out, isErr)
	}

	// A write is recorded as the AI actor, in the pinned workspace.
	ctx := e.ctxFor(alice, a.ID)
	if out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "from mcp"}); isErr {
		t.Fatalf("save_issue: %s", out)
	}
	var actor, ws string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT actor, workspace_id::text FROM activity WHERE issue_title='from mcp' AND kind='created'`).Scan(&actor, &ws); err != nil {
		t.Fatal(err)
	}
	if actor != "ai" || ws != a.ID {
		t.Fatalf("activity actor=%q workspace=%q, want ai in %q", actor, ws, a.ID)
	}
}
