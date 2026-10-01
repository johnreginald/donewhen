package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// PP-186: REST writes commit_linked and criterion_checked rows.
func TestRESTWritesCommitAndCriterionRows(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	wsp := e.workspace(owner)
	opt := browser(e.session(owner))

	w := e.do("POST", "/api/issues", map[string]any{"title": "rest rows", "stateName": "In Progress"}, opt)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var is models.Issue
	_ = json.Unmarshal(w.Body.Bytes(), &is)
	rows := func(kind string) []models.Activity {
		all, err := e.store.ListActivity(t.Context(), wsp.ID, is.ID)
		if err != nil {
			t.Fatal(err)
		}
		var got []models.Activity
		for _, a := range all {
			if a.Kind == kind {
				got = append(got, a)
			}
		}
		return got
	}

	if w = e.do("POST", "/api/issues/"+is.Key+"/commits", map[string]any{"sha": "cafe1234", "message": "m"}, opt); w.Code != http.StatusCreated {
		t.Fatalf("commit: %d %s", w.Code, w.Body.String())
	}
	if c := rows("commit_linked"); len(c) != 1 || c[0].ToVal != "cafe1234" || c[0].Actor != "human" {
		t.Fatalf("commit rows = %+v", c)
	}
	if rows("committed") != nil {
		t.Fatal("the old committed kind must not be written any more")
	}

	w = e.do("POST", "/api/issues/"+is.ID+"/criteria", map[string]any{"body": "ship it"}, opt)
	var crit models.Criterion
	_ = json.Unmarshal(w.Body.Bytes(), &crit)
	if n := len(rows("criterion_checked")); n != 0 {
		t.Fatalf("adding an unticked criterion wrote %d rows", n)
	}
	for i, done := range []bool{true, false} {
		if w = e.do("PATCH", "/api/criteria/"+crit.ID, map[string]any{"done": done}, opt); w.Code != 200 {
			t.Fatalf("patch: %d %s", w.Code, w.Body.String())
		}
		c := rows("criterion_checked")
		if len(c) != i+1 || c[0].FromVal != "ship it" || (c[0].ToVal == "true") != done {
			t.Fatalf("after done=%v rows = %+v", done, c)
		}
	}
	// Re-sending the current value changes nothing.
	if w = e.do("PATCH", "/api/criteria/"+crit.ID, map[string]any{"done": false}, opt); w.Code != 200 {
		t.Fatalf("patch: %d", w.Code)
	}
	if n := len(rows("criterion_checked")); n != 2 {
		t.Fatalf("unchanged patch: %d rows", n)
	}
}
