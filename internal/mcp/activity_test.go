package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-186: link_commit, check_criterion and set_criteria each leave their
// timeline row, written by the store in the same transaction.
func TestMCPWritesCommitAndCriterionRows(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)
	bg := context.Background()

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "rows", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	if err := json.Unmarshal([]byte(out), &is); err != nil {
		t.Fatal(err)
	}
	kinds := func(kind string) []models.Activity {
		rows, err := e.store.ListActivity(bg, w.ID, is.ID)
		if err != nil {
			t.Fatal(err)
		}
		var got []models.Activity
		for _, r := range rows {
			if r.Kind == kind {
				got = append(got, r)
			}
		}
		return got
	}

	if out, isErr = e.call(ctx, "link_commit", map[string]any{"issue": is.Key, "sha": "deadbeef1", "message": "m"}); isErr {
		t.Fatal(out)
	}
	if c := kinds("commit_linked"); len(c) != 1 || c[0].ToVal != "deadbeef1" || c[0].Actor != "ai" {
		t.Fatalf("commit rows = %+v", c)
	}

	if out, isErr = e.call(ctx, "set_criteria", map[string]any{"issue": is.Key, "items": []any{"first", "second"}}); isErr {
		t.Fatal(out)
	}
	if n := len(kinds("criterion_checked")); n != 0 {
		t.Fatalf("set_criteria with nothing ticked wrote %d rows", n)
	}

	// check_criterion ticks one: one row, done true.
	if out, isErr = e.call(ctx, "check_criterion", map[string]any{"issue": is.Key, "index": 1}); isErr {
		t.Fatal(out)
	}
	if c := kinds("criterion_checked"); len(c) != 1 || c[0].FromVal != "first" || c[0].ToVal != "true" || c[0].Actor != "ai" {
		t.Fatalf("check rows = %+v", c)
	}

	// set_criteria flips one item: one more row with that item's text.
	if out, isErr = e.call(ctx, "set_criteria", map[string]any{"issue": is.Key, "items": []any{
		map[string]any{"text": "first", "done": true}, map[string]any{"text": "second", "done": true},
	}}); isErr {
		t.Fatal(out)
	}
	if c := kinds("criterion_checked"); len(c) != 2 || c[0].FromVal != "second" || c[0].ToVal != "true" {
		t.Fatalf("flip rows = %+v", c)
	}

	// Same list again: nothing new.
	if out, isErr = e.call(ctx, "set_criteria", map[string]any{"issue": is.Key, "items": []any{
		map[string]any{"text": "first", "done": true}, map[string]any{"text": "second", "done": true},
	}}); isErr {
		t.Fatal(out)
	}
	if n := len(kinds("criterion_checked")); n != 2 {
		t.Fatalf("unchanged resend: %d rows", n)
	}
}
