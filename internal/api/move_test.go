package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-198: POST /api/issues/{id}/move changes column and rank in one request.

func TestMoveIssue(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	e.workspace(owner)
	opt := browser(e.session(owner))

	create := func(state string) models.Issue {
		t.Helper()
		w := e.do("POST", "/api/issues", map[string]any{"title": "card", "stateName": state}, opt)
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var is models.Issue
		_ = json.Unmarshal(w.Body.Bytes(), &is)
		return is
	}
	move := func(id string, body map[string]any) (int, models.Issue) {
		t.Helper()
		w := e.do("POST", "/api/issues/"+id+"/move", body, opt)
		var is models.Issue
		_ = json.Unmarshal(w.Body.Bytes(), &is)
		return w.Code, is
	}
	// column returns the issue ids of one state in board order.
	column := func(state string) []string {
		t.Helper()
		var states []models.WorkflowState
		_ = json.Unmarshal(e.do("GET", "/api/states", nil, opt).Body.Bytes(), &states)
		var sid string
		for _, s := range states {
			if s.Name == state {
				sid = s.ID
			}
		}
		var list []models.Issue
		_ = json.Unmarshal(e.do("GET", "/api/issues", nil, opt).Body.Bytes(), &list)
		var out []string
		for _, i := range list {
			if i.StateID == sid {
				out = append(out, i.ID)
			}
		}
		return out
	}
	activityCount := func(id string) int {
		var n int
		if err := e.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM activity WHERE issue_id=$1 AND kind='state_changed'`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	a, c := create("Backlog"), create("Backlog")
	create("Backlog")
	d := create("Ready")

	// Into another column, between a and b: one request, one activity row.
	code, got := move(a.ID, map[string]any{"state": "Ready", "after": d.ID, "before": nil})
	if code != 200 {
		t.Fatalf("move: %d", code)
	}
	if got.StateID == a.StateID {
		t.Fatal("state did not change")
	}
	if want := []string{d.ID, a.ID}; !slices.Equal(column("Ready"), want) {
		t.Fatalf("Ready = %v, want %v", column("Ready"), want)
	}
	if n := activityCount(a.ID); n != 1 {
		t.Fatalf("state_changed rows = %d, want 1", n)
	}

	// Reorder inside the column, no state: between b and c.
	create("Backlog")
	e1 := column("Backlog")
	if code, _ := move(e1[2], map[string]any{"after": e1[0], "before": e1[1]}); code != 200 {
		t.Fatalf("reorder: %d", code)
	}
	if want := []string{e1[0], e1[2], e1[1]}; !slices.Equal(column("Backlog"), want) {
		t.Fatalf("Backlog = %v, want %v", column("Backlog"), want)
	}

	// A neighbour in another column is refused, and nothing moves.
	if code, _ := move(c.ID, map[string]any{"state": "Backlog", "after": d.ID}); code != 400 {
		t.Fatalf("foreign neighbour: %d, want 400", code)
	}
	// So is an unknown state.
	if code, _ := move(c.ID, map[string]any{"state": "Nope"}); code != 400 {
		t.Fatalf("unknown state: %d, want 400", code)
	}

	// Squeeze: drop many cards right after the same neighbour until the gap
	// is too small, which renumbers the column. Order must stay exact.
	top := create("Triage")
	rest := create("Triage")
	want := []string{top.ID, rest.ID}
	for range 40 {
		x := create("Backlog")
		if code, _ := move(x.ID, map[string]any{"state": "Triage", "after": top.ID, "before": want[1]}); code != 200 {
			t.Fatalf("squeeze: %d", code)
		}
		want = append([]string{want[0], x.ID}, want[1:]...)
	}
	if got := column("Triage"); !slices.Equal(got, want) {
		t.Fatalf("Triage order = %v, want %v", got, want)
	}
}
