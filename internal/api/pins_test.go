package api

import (
	"context"
	"net/http"
	"testing"

	"raenil/internal/config"
	"raenil/internal/models"
)

// PP-188 part 1: a bearer caller, pinned or not, can never mint tokens or
// create/switch workspaces. Only a browser session may.
func TestBearerCannotManageTokensOrWorkspaces(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	wsp := e.workspace(uid)
	pinned := e.token(uid, wsp.ID)
	unpinned := e.token(uid, "")
	cookies := e.session(uid)

	tokenBody := map[string]string{"name": "x"}
	wsBody := map[string]string{"name": "New " + uniq(), "keyPrefix": "Z" + uniq()}

	cases := []struct {
		name   string
		method string
		path   string
		body   any
		opt    reqOpt
		want   int
	}{
		{"pinned bearer lists tokens", "GET", "/api/tokens", nil, bearer(pinned), 403},
		{"pinned bearer creates token", "POST", "/api/tokens", tokenBody, bearer(pinned), 403},
		{"unpinned bearer creates token", "POST", "/api/tokens", tokenBody, bearer(unpinned), 403},
		{"pinned bearer deletes token", "DELETE", "/api/tokens/" + uniq(), nil, bearer(pinned), 403},
		{"unpinned bearer creates workspace", "POST", "/api/workspaces", wsBody, bearer(unpinned), 403},
		{"pinned bearer creates workspace", "POST", "/api/workspaces", wsBody, bearer(pinned), 403},
		{"pinned bearer activates workspace", "POST", "/api/workspaces/" + wsp.ID + "/activate", nil, bearer(pinned), 403},
		{"unpinned bearer activates workspace", "POST", "/api/workspaces/" + wsp.ID + "/activate", nil, bearer(unpinned), 403},
		{"browser creates token", "POST", "/api/tokens", tokenBody, browser(cookies), 201},
		{"browser lists tokens", "GET", "/api/tokens", nil, browser(cookies), 200},
		{"browser activates workspace", "POST", "/api/workspaces/" + wsp.ID + "/activate", nil, browser(cookies), 200},
		{"browser creates workspace", "POST", "/api/workspaces", wsBody, browser(cookies), 201},
		{"no credentials", "POST", "/api/tokens", tokenBody, nil, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []reqOpt
			if tc.opt != nil {
				opts = append(opts, tc.opt)
			}
			w := e.do(tc.method, tc.path, tc.body, opts...)
			if w.Code != tc.want {
				t.Fatalf("%s %s = %d (%s), want %d", tc.method, tc.path, w.Code, w.Body.String(), tc.want)
			}
			if tc.want == 403 && errBody(w) != "forbidden" {
				t.Fatalf("error = %q, want forbidden", errBody(w))
			}
		})
	}
	// Clean up the workspace the browser case created.
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), `DELETE FROM workspaces WHERE key_prefix = upper($1)`, wsBody["keyPrefix"])
	})
}

// PP-188 part 2 (REST): a pinned token stops working in the pinned workspace
// once its owner is no longer a member, and cannot be pointed at another one.
func TestPinnedTokenRecheckedAgainstMembership(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	a := e.workspace(uid)
	b := e.workspace(uid)
	tok := e.token(uid, a.ID)

	if w := e.do("GET", "/api/issues", nil, bearer(tok)); w.Code != 200 {
		t.Fatalf("pinned member: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("GET", "/api/issues", nil, bearer(tok), header("X-Workspace", b.ID)); w.Code != 403 {
		t.Fatalf("pinned token naming another workspace: %d, want 403", w.Code)
	}

	// Drop the membership behind the store's back (RemoveMember would also
	// delete the token, which is the other test); the pin path must still refuse.
	if _, err := e.pool.Exec(context.Background(),
		`DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, a.ID, uid); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/issues", nil, bearer(tok)); w.Code != 403 {
		t.Fatalf("pinned non-member: %d %s, want 403", w.Code, w.Body.String())
	}
}

// PP-188 part 3: removing a member deletes their tokens pinned to that
// workspace and keeps the rest.
func TestRemoveMemberRevokesPinnedTokens(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	member, _ := e.user("")
	a := e.workspace(owner)
	b := e.workspace(owner)
	ctx := context.Background()
	for _, w := range []models.Workspace{a, b} {
		if err := e.store.AddMember(ctx, w.ID, member, models.RoleMember); err != nil {
			t.Fatal(err)
		}
	}
	pinA := e.token(member, a.ID)
	pinB := e.token(member, b.ID)
	unpinned := e.token(member, "")
	ownerPinA := e.token(owner, a.ID)
	cookies := e.session(owner)

	w := e.do("DELETE", "/api/workspaces/"+a.ID+"/members/"+member, nil,
		browser(cookies), header("X-Workspace", a.ID))
	if w.Code != 200 {
		t.Fatalf("remove member: %d %s", w.Code, w.Body.String())
	}

	// Authenticated routes answer 401 for a token that no longer exists.
	cases := []struct {
		name string
		tok  string
		want int
	}{
		{"removed member's token pinned to A is gone", pinA, 401},
		{"same member's token pinned to B survives", pinB, 200},
		{"same member's unpinned token survives", unpinned, 200},
		{"another user's token pinned to A survives", ownerPinA, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hdr := []reqOpt{bearer(tc.tok)}
			if tc.tok == unpinned {
				hdr = append(hdr, header("X-Workspace", b.ID))
			}
			if got := e.do("GET", "/api/me", nil, hdr...).Code; got != tc.want {
				t.Fatalf("GET /api/me = %d, want %d", got, tc.want)
			}
		})
	}
	if got := e.do("GET", "/api/issues", nil, bearer(pinA)); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token reached a workspace route: %d", got.Code)
	}
}
