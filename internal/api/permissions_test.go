package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-201: the permission layers of the REST API, end to end through the real
// handler chain: guard (401, CSRF), wsGuard (workspace resolution), adminOnly,
// and the 404 a caller gets for another workspace's rows.

// idOf decodes {"id": ...} from a response.
func idOf(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.ID == "" {
		t.Fatalf("no id in %s (%v)", body, err)
	}
	return v.ID
}

func TestGuardRejectsMissingOrBadCredentials(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	e.workspace(uid)

	cases := []struct {
		name string
		opts []reqOpt
	}{
		{"no credentials", nil},
		{"unknown bearer token", []reqOpt{bearer("donewhen_nope")}},
		{"empty bearer token", []reqOpt{header("Authorization", "Bearer ")}},
		{"unknown session cookie", []reqOpt{func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "donewhen_session", Value: "nope"})
		}}},
	}
	for _, tc := range cases {
		for _, route := range []struct{ method, path string }{
			{"GET", "/api/me"},
			{"GET", "/api/issues"},
			{"POST", "/api/issues"},
			{"GET", "/api/workspaces"},
			{"GET", "/api/events"},
		} {
			t.Run(tc.name+" "+route.method+" "+route.path, func(t *testing.T) {
				w := e.do(route.method, route.path, nil, tc.opts...)
				if w.Code != http.StatusUnauthorized {
					t.Fatalf("= %d (%s), want 401", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestCSRFAppliesToCookiesNotBearer(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	wsp := e.workspace(uid)
	cookies := e.session(uid)
	tok := e.token(uid, wsp.ID)
	body := map[string]any{"title": "csrf"}

	// Cookie session, state-changing, no/wrong CSRF header → 403.
	noHeader := func(r *http.Request) {
		for _, c := range cookies {
			r.AddCookie(c)
		}
	}
	wrongHeader := func(r *http.Request) {
		noHeader(r)
		r.Header.Set("X-CSRF-Token", "not-the-cookie-value")
	}
	for name, opt := range map[string]reqOpt{"missing header": noHeader, "wrong header": wrongHeader} {
		w := e.do("POST", "/api/issues", body, opt)
		if w.Code != http.StatusForbidden || errBody(w) != "invalid csrf token" {
			t.Fatalf("cookie POST with %s = %d %s, want 403 invalid csrf token", name, w.Code, w.Body.String())
		}
	}
	// A header with no cookie to match is also refused.
	if w := e.do("POST", "/api/issues", body, func(r *http.Request) {
		r.AddCookie(cookies[0])
		r.Header.Set("X-CSRF-Token", "x")
	}); w.Code != http.StatusForbidden {
		t.Fatalf("cookie POST with header but no csrf cookie = %d, want 403", w.Code)
	}
	// Matching double-submit token → allowed.
	if w := e.do("POST", "/api/issues", body, browser(cookies)); w.Code != http.StatusCreated {
		t.Fatalf("cookie POST with csrf = %d %s, want 201", w.Code, w.Body.String())
	}
	// Reads never need the token.
	if w := e.do("GET", "/api/issues", nil, noHeader); w.Code != http.StatusOK {
		t.Fatalf("cookie GET without csrf = %d, want 200", w.Code)
	}
	// A bearer token carries no ambient credential, so it needs no CSRF token.
	if w := e.do("POST", "/api/issues", body, bearer(tok)); w.Code != http.StatusCreated {
		t.Fatalf("bearer POST without csrf = %d %s, want 201", w.Code, w.Body.String())
	}
}

func TestWsGuardWorkspaceResolutionMapping(t *testing.T) {
	e := newEnv(t, config.Config{})
	alice, _ := e.user("")
	bob, _ := e.user("")
	loner, _ := e.user("") // member of nothing
	a := e.workspace(alice)
	b := e.workspace(bob)
	a2 := e.workspace(alice) // alice now spans two workspaces

	aliceSession := browser(e.session(alice))
	aliceUnpinned := bearer(e.token(alice, ""))
	alicePinnedA := bearer(e.token(alice, a.ID))

	cases := []struct {
		name string
		opts []reqOpt
		want int
	}{
		{"names own workspace by id", []reqOpt{aliceSession, header("X-Workspace", a.ID)}, 200},
		{"names own workspace by slug", []reqOpt{aliceSession, header("X-Workspace", a.Slug)}, 200},
		{"names own workspace by prefix", []reqOpt{aliceSession, header("X-Workspace", a.KeyPrefix)}, 200},
		{"names a workspace she is not in", []reqOpt{aliceSession, header("X-Workspace", b.ID)}, 403},
		{"names an unknown workspace", []reqOpt{aliceSession, header("X-Workspace", "no-such-workspace")}, 404},
		{"query parameter names the workspace", []reqOpt{aliceSession}, 200}, // see below
		{"no membership at all", []reqOpt{browser(e.session(loner))}, 403},
		{"unpinned token, two workspaces, none named", []reqOpt{aliceUnpinned}, 400},
		{"unpinned token names one", []reqOpt{aliceUnpinned, header("X-Workspace", a2.ID)}, 200},
		{"pinned token, none named", []reqOpt{alicePinnedA}, 200},
		{"pinned token names its own", []reqOpt{alicePinnedA, header("X-Workspace", a.ID)}, 200},
		{"pinned token names her other workspace", []reqOpt{alicePinnedA, header("X-Workspace", a2.ID)}, 403},
		{"pinned token names a stranger's workspace", []reqOpt{alicePinnedA, header("X-Workspace", b.ID)}, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/issues"
			if strings.HasPrefix(tc.name, "query parameter") {
				path += "?workspace=" + a.ID
			}
			w := e.do("GET", path, nil, tc.opts...)
			if w.Code != tc.want {
				t.Fatalf("GET %s = %d (%s), want %d", path, w.Code, w.Body.String(), tc.want)
			}
		})
	}

	// The query parameter loses to nothing here, but it must still be checked.
	if w := e.do("GET", "/api/issues?workspace="+b.ID, nil, aliceSession); w.Code != 403 {
		t.Fatalf("?workspace= another workspace = %d, want 403", w.Code)
	}
}

func TestAdminOnlyRefusesMembers(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	admin, _ := e.user("")
	member, _ := e.user("")
	target, targetEmail := e.user("")
	wsp := e.workspace(owner)
	ctx := context.Background()
	if err := e.store.AddMember(ctx, wsp.ID, admin, models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := e.store.AddMember(ctx, wsp.ID, member, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	ws := header("X-Workspace", wsp.ID)

	type route struct {
		method, path string
		body         any
	}
	routes := []route{
		{"PATCH", "/api/workspaces/" + wsp.ID, map[string]any{"name": "Renamed " + uniq()}},
		{"POST", "/api/workspaces/" + wsp.ID + "/members", map[string]string{"email": targetEmail, "role": "member"}},
		{"DELETE", "/api/workspaces/" + wsp.ID + "/members/" + target, nil},
		{"POST", "/api/import/descriptions", map[string]string{"NOPE-1": "x"}},
	}
	memberB := browser(e.session(member))
	adminB := browser(e.session(admin))
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			w := e.do(rt.method, rt.path, rt.body, memberB, ws)
			if w.Code != http.StatusForbidden || errBody(w) != "requires workspace owner or admin" {
				t.Fatalf("member = %d %s, want 403 requires workspace owner or admin", w.Code, w.Body.String())
			}
			w = e.do(rt.method, rt.path, rt.body, adminB, ws)
			if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
				t.Fatalf("admin = %d %s, want it allowed", w.Code, w.Body.String())
			}
		})
	}
}

// A member can read tenant data but not administer; the same member's bearer
// token gets the same answer.
func TestMemberBearerCannotAdminister(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	member, _ := e.user("")
	wsp := e.workspace(owner)
	if err := e.store.AddMember(context.Background(), wsp.ID, member, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	tok := bearer(e.token(member, wsp.ID))
	if w := e.do("GET", "/api/issues", nil, tok); w.Code != 200 {
		t.Fatalf("member read = %d", w.Code)
	}
	w := e.do("PATCH", "/api/workspaces/"+wsp.ID, map[string]any{"name": "x"}, tok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member bearer PATCH workspace = %d %s, want 403", w.Code, w.Body.String())
	}
	got, _ := e.store.GetWorkspace(context.Background(), wsp.ID)
	if got.Name != wsp.Name {
		t.Fatalf("workspace renamed to %q", got.Name)
	}
}

// Rows of one workspace are invisible and untouchable from another, by uuid and
// by human key, and a caller cannot reach them by naming the other workspace.
func TestCrossWorkspaceAccessIs404(t *testing.T) {
	e := newEnv(t, config.Config{})
	alice, _ := e.user("")
	bob, _ := e.user("")
	a := e.workspace(alice)
	b := e.workspace(bob)
	bobB := browser(e.session(bob))
	bobWS := header("X-Workspace", b.ID)

	// Bob seeds workspace B.
	w := e.do("POST", "/api/issues", map[string]any{"title": "bob secret"}, bobB, bobWS)
	if w.Code != 201 {
		t.Fatalf("seed issue: %d %s", w.Code, w.Body.String())
	}
	issueID := idOf(t, w.Body.Bytes())
	var seeded models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &seeded)
	key := seeded.Key
	w = e.do("POST", "/api/issues/"+issueID+"/criteria", map[string]any{"body": "c1"}, bobB, bobWS)
	if w.Code != 201 {
		t.Fatalf("seed criterion: %d %s", w.Code, w.Body.String())
	}
	critID := idOf(t, w.Body.Bytes())
	w = e.do("POST", "/api/projects", map[string]any{"name": "bob epic"}, bobB, bobWS)
	if w.Code != 200 {
		t.Fatalf("seed project: %d %s", w.Code, w.Body.String())
	}
	projID := idOf(t, w.Body.Bytes())
	w = e.do("POST", "/api/documents", map[string]any{"title": "bob doc", "bodyMd": "x"}, bobB, bobWS)
	if w.Code != 200 && w.Code != 201 {
		t.Fatalf("seed document: %d %s", w.Code, w.Body.String())
	}
	docID := idOf(t, w.Body.Bytes())

	aliceB := browser(e.session(alice))
	aliceTok := bearer(e.token(alice, a.ID))
	callers := map[string][]reqOpt{
		"session in A": {aliceB, header("X-Workspace", a.ID)},
		"pinned token": {aliceTok},
	}

	type route struct {
		method, path string
		body         any
	}
	routes := []route{
		{"GET", "/api/issues/" + issueID, nil},
		{"GET", "/api/issues/" + key, nil},
		{"PATCH", "/api/issues/" + issueID, map[string]any{"title": "hacked"}},
		{"PATCH", "/api/issues/" + key, map[string]any{"title": "hacked"}},
		{"DELETE", "/api/issues/" + issueID, nil},
		{"DELETE", "/api/issues/" + key, nil},
		{"GET", "/api/issues/" + key + "/activity", nil},
		{"GET", "/api/issues/" + issueID + "/comments", nil},
		{"POST", "/api/issues/" + key + "/comments", map[string]any{"bodyMd": "hi"}},
		{"GET", "/api/issues/" + key + "/criteria", nil},
		{"POST", "/api/issues/" + key + "/criteria", map[string]any{"body": "mine now"}},
		{"GET", "/api/issues/" + key + "/commits", nil},
		{"POST", "/api/issues/" + key + "/commits", map[string]any{"sha": "abcdef1234567"}},
		{"PATCH", "/api/issues/" + key + "/dev", map[string]any{"gitBranch": "x"}},
		{"GET", "/api/issues/" + key + "/blockers", nil},
		{"PATCH", "/api/criteria/" + critID, map[string]any{"done": true}},
		{"DELETE", "/api/criteria/" + critID, nil},
		{"GET", "/api/projects/" + projID, nil},
		{"PATCH", "/api/projects/" + projID, map[string]any{"name": "hacked"}},
		{"DELETE", "/api/projects/" + projID, nil},
		{"GET", "/api/documents/" + docID, nil},
		{"DELETE", "/api/documents/" + docID, nil},
	}
	for who, opts := range callers {
		for _, rt := range routes {
			t.Run(who+" "+rt.method+" "+rt.path, func(t *testing.T) {
				w := e.do(rt.method, rt.path, rt.body, opts...)
				if w.Code != http.StatusNotFound {
					t.Fatalf("= %d (%s), want 404", w.Code, w.Body.String())
				}
			})
		}
	}

	// Lists show only the caller's own workspace.
	for who, opts := range callers {
		for _, path := range []string{"/api/issues", "/api/projects", "/api/documents", "/api/activity"} {
			w := e.do("GET", path, nil, opts...)
			if w.Code != 200 {
				t.Fatalf("%s GET %s = %d", who, path, w.Code)
			}
			if strings.Contains(w.Body.String(), "bob secret") || strings.Contains(w.Body.String(), "bob epic") ||
				strings.Contains(w.Body.String(), "bob doc") {
				t.Fatalf("%s GET %s leaked B's rows: %s", who, path, w.Body.String())
			}
		}
	}

	// Naming B directly is a membership failure, not a way in.
	for _, rt := range routes[:6] {
		w := e.do(rt.method, rt.path, rt.body, aliceB, header("X-Workspace", b.ID))
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s naming B = %d, want 403", rt.method, rt.path, w.Code)
		}
	}

	// Nothing of Bob's changed.
	w = e.do("GET", "/api/issues/"+issueID, nil, bobB, bobWS)
	if w.Code != 200 {
		t.Fatalf("bob lost his issue: %d %s", w.Code, w.Body.String())
	}
	var got models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Title != "bob secret" {
		t.Fatalf("bob's issue title = %q", got.Title)
	}
	w = e.do("GET", "/api/issues/"+issueID+"/criteria", nil, bobB, bobWS)
	if strings.Contains(w.Body.String(), "mine now") || !strings.Contains(w.Body.String(), `"done":false`) {
		t.Fatalf("bob's criteria were touched: %s", w.Body.String())
	}
	if w = e.do("GET", "/api/projects/"+projID, nil, bobB, bobWS); w.Code != 200 {
		t.Fatalf("bob lost his project: %d", w.Code)
	}
}
