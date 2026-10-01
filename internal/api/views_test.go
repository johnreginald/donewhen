package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

// PP-205: saved views are private per user, with three defaults on first load.

func TestSavedViews(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	w := e.workspace(owner)
	other, _ := e.user("")
	if err := e.store.AddMember(t.Context(), w.ID, other, "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	a := browser(e.session(owner))
	b := browser(e.session(other))

	list := func(opt reqOpt) []store.View {
		t.Helper()
		r := e.do("GET", "/api/views", nil, opt)
		if r.Code != 200 {
			t.Fatalf("list: %d %s", r.Code, r.Body.String())
		}
		var vs []store.View
		_ = json.Unmarshal(r.Body.Bytes(), &vs)
		return vs
	}

	// First load creates the defaults, in order, as list views.
	vs := list(a)
	want := []string{"In Review", "Blocked", "Ready queue"}
	if len(vs) != 3 {
		t.Fatalf("defaults = %+v", vs)
	}
	for i, n := range want {
		if vs[i].Name != n || vs[i].Layout != "list" {
			t.Fatalf("default %d = %+v", i, vs[i])
		}
	}
	if vs[0].Query != "state=In+Review" {
		t.Fatalf("In Review query = %q", vs[0].Query)
	}

	// Create, validate, patch.
	r := e.do("POST", "/api/views", map[string]any{"name": "Mine", "query": "?priority=1,2", "layout": "board"}, a)
	if r.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", r.Code, r.Body.String())
	}
	var mine store.View
	_ = json.Unmarshal(r.Body.Bytes(), &mine)
	if mine.Query != "priority=1,2" || mine.Layout != "board" || mine.Position != 3 {
		t.Fatalf("mine = %+v", mine)
	}
	for name, body := range map[string]map[string]any{
		"empty name": {"name": "  ", "layout": "list"},
		"bad layout": {"name": "x", "layout": "grid"},
		"long name":  {"name": "0123456789012345678901234567890123456789012345678901234567890", "layout": "list"},
	} {
		if r := e.do("POST", "/api/views", body, a); r.Code != 400 {
			t.Fatalf("%s: %d", name, r.Code)
		}
	}
	r = e.do("PATCH", "/api/views/"+mine.ID, map[string]any{"name": "Mine now", "layout": "list"}, a)
	if r.Code != 200 {
		t.Fatalf("patch: %d %s", r.Code, r.Body.String())
	}

	// User B sees only their own (defaults), and cannot touch A's view.
	for _, v := range list(b) {
		if v.ID == mine.ID {
			t.Fatal("B can see A's view")
		}
	}
	if r := e.do("PATCH", "/api/views/"+mine.ID, map[string]any{"name": "stolen"}, b); r.Code != 404 {
		t.Fatalf("B patch: %d", r.Code)
	}
	if r := e.do("DELETE", "/api/views/"+mine.ID, nil, b); r.Code != 404 {
		t.Fatalf("B delete: %d", r.Code)
	}

	// A deletes a default; it does not come back.
	if r := e.do("DELETE", "/api/views/"+vs[1].ID, nil, a); r.Code != 204 {
		t.Fatalf("delete: %d", r.Code)
	}
	for _, v := range list(a) {
		if v.Name == "Blocked" {
			t.Fatal("deleted default came back")
		}
	}
	if got := len(list(a)); got != 3 { // In Review, Ready queue, Mine now
		t.Fatalf("A has %d views", got)
	}
}

// PP-205: GET /api/issues filters by state, priority and label, and a text
// search treats % and _ literally.
func TestIssueFilters(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	opt := browser(e.session(owner))

	mk := func(body map[string]any) models.Issue {
		t.Helper()
		r := e.do("POST", "/api/issues", body, opt)
		if r.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", r.Code, r.Body.String())
		}
		var is models.Issue
		_ = json.Unmarshal(r.Body.Bytes(), &is)
		return is
	}
	var labels []models.Label
	_ = json.Unmarshal(e.do("GET", "/api/labels", nil, opt).Body.Bytes(), &labels)
	if len(labels) < 2 {
		t.Fatalf("need two seeded labels, got %d", len(labels))
	}
	a := mk(map[string]any{"title": "alpha 50% done", "stateName": "Ready", "priority": 1, "labelIds": []string{labels[0].ID}})
	b := mk(map[string]any{"title": "beta snake_case", "stateName": "Backlog", "priority": 2, "labelIds": []string{labels[1].ID}})
	c := mk(map[string]any{"title": "gamma 50x done", "stateName": "Ready", "priority": 0})

	keys := func(q url.Values) map[string]bool {
		t.Helper()
		r := e.do("GET", "/api/issues?"+q.Encode(), nil, opt)
		if r.Code != 200 {
			t.Fatalf("%v: %d %s", q, r.Code, r.Body.String())
		}
		var list []models.Issue
		_ = json.Unmarshal(r.Body.Bytes(), &list)
		out := map[string]bool{}
		for _, i := range list {
			out[i.Key] = true
		}
		return out
	}
	same := func(got map[string]bool, want ...models.Issue) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %v, want %d issues", got, len(want))
		}
		for _, w := range want {
			if !got[w.Key] {
				t.Fatalf("got %v, missing %s", got, w.Key)
			}
		}
	}

	same(keys(url.Values{"state": {"Ready"}}), a, c)
	same(keys(url.Values{"state": {"ready,Backlog"}}), a, b, c)
	same(keys(url.Values{"state": {a.StateID}}), a, c)
	same(keys(url.Values{"priority": {"1,2"}}), a, b)
	same(keys(url.Values{"priority": {"0"}}), c)
	same(keys(url.Values{"label": {labels[0].ID + "," + labels[1].ID}}), a, b)
	same(keys(url.Values{"state": {"Ready"}, "priority": {"1"}}), a) // AND across filters
	if r := e.do("GET", "/api/issues?priority=high", nil, opt); r.Code != 400 {
		t.Fatalf("bad priority: %d", r.Code)
	}

	// % and _ are literal: "50%" does not match "50x", "snake_" does not match "snakeX".
	same(keys(url.Values{"q": {"50%"}}), a)
	same(keys(url.Values{"q": {"snake_c"}}), b)
	same(keys(url.Values{"q": {"_"}}), b)
}
