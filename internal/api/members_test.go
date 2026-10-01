package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-190: member routes act on the workspace in the URL, with safe owner rules.

func memberRole(t *testing.T, e *testEnv, wsID, user string) string {
	t.Helper()
	role, err := e.store.RoleIn(context.Background(), wsID, user)
	if err != nil {
		return ""
	}
	return role
}

func TestMemberRoutesActOnPathWorkspace(t *testing.T) {
	e := newEnv(t, config.Config{})
	admin, _ := e.user("")
	target, targetEmail := e.user("")
	boss, _ := e.user("")
	a := e.workspace(admin) // admin owns A, which is the active workspace
	b := e.workspace(boss)  // admin is only an admin of B
	ctx := context.Background()
	if err := e.store.AddMember(ctx, b.ID, admin, models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := e.store.AddMember(ctx, b.ID, target, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	cookies := e.session(admin)

	// Active workspace is A, but the URL names B (by slug): list acts on B.
	w := e.do("GET", "/api/workspaces/"+b.Slug+"/members", nil, browser(cookies), header("X-Workspace", a.ID))
	if w.Code != 200 {
		t.Fatalf("list B members: %d %s", w.Code, w.Body.String())
	}
	var members []models.Member
	if err := json.Unmarshal(w.Body.Bytes(), &members); err != nil || len(members) != 3 {
		t.Fatalf("list returned %d members (%v), want B's 3", len(members), err)
	}

	// Remove acts on B, not A.
	w = e.do("DELETE", "/api/workspaces/"+b.ID+"/members/"+target, nil, browser(cookies), header("X-Workspace", a.ID))
	if w.Code != 200 {
		t.Fatalf("remove from B: %d %s", w.Code, w.Body.String())
	}
	if memberRole(t, e, b.ID, target) != "" {
		t.Fatal("target should be gone from B")
	}

	// Add acts on B (by prefix) too.
	w = e.do("POST", "/api/workspaces/"+b.KeyPrefix+"/members",
		map[string]string{"email": targetEmail, "role": "member"}, browser(cookies), header("X-Workspace", a.ID))
	if w.Code != 201 {
		t.Fatalf("add to B: %d %s", w.Code, w.Body.String())
	}
	if memberRole(t, e, b.ID, target) != models.RoleMember {
		t.Fatal("target should be a member of B")
	}
	if memberRole(t, e, a.ID, target) != "" {
		t.Fatal("A must not have been touched")
	}
}

func TestMemberRoutesRoleCheckUsesPathWorkspace(t *testing.T) {
	e := newEnv(t, config.Config{})
	user, _ := e.user("")
	victim, _ := e.user("")
	boss, _ := e.user("")
	a := e.workspace(user) // owner of the active workspace A
	b := e.workspace(boss) // plain member of B
	ctx := context.Background()
	if err := e.store.AddMember(ctx, b.ID, user, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := e.store.AddMember(ctx, b.ID, victim, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	cookies := e.session(user)

	// Owner of A must not gain admin rights over B through the active workspace.
	w := e.do("DELETE", "/api/workspaces/"+b.ID+"/members/"+victim, nil, browser(cookies), header("X-Workspace", a.ID))
	if w.Code != 403 {
		t.Fatalf("member of B removing: %d, want 403", w.Code)
	}
	if memberRole(t, e, b.ID, victim) == "" {
		t.Fatal("victim must still be in B")
	}
}

func TestMemberRoutesHideUnknownAndForeignWorkspaces(t *testing.T) {
	e := newEnv(t, config.Config{})
	user, _ := e.user("")
	other, _ := e.user("")
	mine := e.workspace(user)
	foreign := e.workspace(other)
	cookies := e.session(user)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/workspaces/" + foreign.ID + "/members"},
		{"GET", "/api/workspaces/no-such-workspace/members"},
		{"DELETE", "/api/workspaces/" + foreign.ID + "/members/" + other},
		{"DELETE", "/api/workspaces/no-such-workspace/members/" + other},
	} {
		w := e.do(tc.method, tc.path, nil, browser(cookies), header("X-Workspace", mine.ID))
		if w.Code != 404 {
			t.Fatalf("%s %s = %d, want 404", tc.method, tc.path, w.Code)
		}
	}
	w := e.do("POST", "/api/workspaces/"+foreign.ID+"/members", map[string]string{"email": "x@example.test"},
		browser(cookies), header("X-Workspace", mine.ID))
	if w.Code != 404 {
		t.Fatalf("add to foreign workspace = %d, want 404", w.Code)
	}
	if memberRole(t, e, foreign.ID, other) != models.RoleOwner {
		t.Fatal("foreign owner must be untouched")
	}
}

func TestMemberRoleRules(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	admin, _ := e.user("")
	member, _ := e.user("")
	peon, peonEmail := e.user("")
	coOwner, coOwnerEmail := e.user("")
	w0 := e.workspace(owner)
	ctx := context.Background()
	for u, r := range map[string]string{admin: models.RoleAdmin, member: models.RoleMember, coOwner: models.RoleOwner} {
		if err := e.store.AddMember(ctx, w0.ID, u, r); err != nil {
			t.Fatal(err)
		}
	}
	base := "/api/workspaces/" + w0.ID + "/members"
	as := func(u string) reqOpt { return browser(e.session(u)) }

	t.Run("admin cannot grant owner", func(t *testing.T) {
		w := e.do("POST", base, map[string]string{"email": peonEmail, "role": "owner"}, as(admin))
		if w.Code != 403 {
			t.Fatalf("got %d, want 403", w.Code)
		}
		if memberRole(t, e, w0.ID, peon) != "" {
			t.Fatal("nothing should have been granted")
		}
	})
	t.Run("admin cannot change an owner's role", func(t *testing.T) {
		w := e.do("POST", base, map[string]string{"email": coOwnerEmail, "role": "member"}, as(admin))
		if w.Code != 403 {
			t.Fatalf("got %d, want 403", w.Code)
		}
		if memberRole(t, e, w0.ID, coOwner) != models.RoleOwner {
			t.Fatal("owner role must be unchanged")
		}
	})
	t.Run("admin cannot remove an owner", func(t *testing.T) {
		w := e.do("DELETE", base+"/"+coOwner, nil, as(admin))
		if w.Code != 403 {
			t.Fatalf("got %d, want 403", w.Code)
		}
	})
	t.Run("member cannot change members", func(t *testing.T) {
		w := e.do("POST", base, map[string]string{"email": peonEmail, "role": "member"}, as(member))
		if w.Code != 403 {
			t.Fatalf("add: got %d, want 403", w.Code)
		}
		w = e.do("DELETE", base+"/"+admin, nil, as(member))
		if w.Code != 403 {
			t.Fatalf("remove: got %d, want 403", w.Code)
		}
	})
	t.Run("admin can add and remove a member or admin", func(t *testing.T) {
		for _, role := range []string{"member", "admin"} {
			w := e.do("POST", base, map[string]string{"email": peonEmail, "role": role}, as(admin))
			if w.Code != 201 {
				t.Fatalf("add %s: got %d %s", role, w.Code, w.Body.String())
			}
		}
		w := e.do("DELETE", base+"/"+peon, nil, as(admin))
		if w.Code != 200 {
			t.Fatalf("remove: got %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("owner can grant and remove owner", func(t *testing.T) {
		w := e.do("POST", base, map[string]string{"email": peonEmail, "role": "owner"}, as(owner))
		if w.Code != 201 || memberRole(t, e, w0.ID, peon) != models.RoleOwner {
			t.Fatalf("grant owner: got %d %s", w.Code, w.Body.String())
		}
		w = e.do("DELETE", base+"/"+peon, nil, as(owner))
		if w.Code != 200 {
			t.Fatalf("remove owner: got %d %s", w.Code, w.Body.String())
		}
	})
}

func TestLastOwnerReturns409(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, ownerEmail := e.user("")
	w0 := e.workspace(owner)
	base := "/api/workspaces/" + w0.ID + "/members"
	cookies := browser(e.session(owner))

	w := e.do("POST", base, map[string]string{"email": ownerEmail, "role": "admin"}, cookies)
	if w.Code != 409 {
		t.Fatalf("demote sole owner: %d, want 409", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["code"] != "last_owner" {
		t.Fatalf("code = %q, want last_owner", body["code"])
	}
	w = e.do("DELETE", base+"/"+owner, nil, cookies)
	if w.Code != 409 {
		t.Fatalf("remove sole owner: %d, want 409", w.Code)
	}
	if memberRole(t, e, w0.ID, owner) != models.RoleOwner {
		t.Fatal("sole owner must remain")
	}
}
