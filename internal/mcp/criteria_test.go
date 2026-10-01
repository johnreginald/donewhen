package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-184: set_criteria rejects bad input before any write and replaces the
// list atomically.
func TestSetCriteriaValidationAndReplace(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)
	bg := context.Background()

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "criteria", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	if err := json.Unmarshal([]byte(out), &is); err != nil {
		t.Fatal(err)
	}
	set := func(items any) (string, bool) {
		return e.call(ctx, "set_criteria", map[string]any{"issue": is.Key, "items": items})
	}
	stored := func() []string {
		cs, err := e.store.ListCriteria(bg, w.ID, is.ID)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for i, c := range cs {
			if c.Position != i {
				t.Errorf("position %d, want %d", c.Position, i)
			}
			out = append(out, c.Body)
		}
		return out
	}
	same := func(got []string, want ...string) {
		t.Helper()
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("stored %q, want %q", got, want)
		}
	}

	if out, isErr = set([]any{"a", "b", "c"}); isErr {
		t.Fatal(out)
	}
	same(stored(), "a", "b", "c")

	// Bad input: error, list unchanged.
	bad := []struct {
		name  string
		items any
		want  string
	}{
		{"string", "a,b", "items must be an array"},
		{"empty", []any{}, "items must not be empty"},
		{"null", nil, "items"},
		{"blank text", []any{map[string]any{"text": "ok"}, map[string]any{"text": " "}}, "items[1]"},
		{"bad kind", []any{map[string]any{"text": "x", "kind": "magic"}}, "items[0]"},
		{"no check", []any{map[string]any{"text": "ok"}, map[string]any{"text": "x", "kind": "deterministic"}}, "items[1]"},
		{"bad type", []any{"ok", 7}, "items[1]"},
	}
	for _, c := range bad {
		out, isErr := set(c.items)
		if !isErr || !strings.Contains(out, c.want) {
			t.Fatalf("%s: err=%v %s", c.name, isErr, out)
		}
		same(stored(), "a", "b", "c")
	}
	// Missing entirely.
	if out, isErr = e.call(ctx, "set_criteria", map[string]any{"issue": is.Key}); !isErr {
		t.Fatalf("missing items accepted: %s", out)
	}
	same(stored(), "a", "b", "c")

	// 3 stored, send 2 -> exactly 2.
	if out, isErr = set([]any{"a", map[string]any{"text": "b", "done": true}}); isErr {
		t.Fatal(out)
	}
	same(stored(), "a", "b")

	// Same list twice -> no duplicates, ids stable.
	before, _ := e.store.ListCriteria(bg, w.ID, is.ID)
	if out, isErr = set([]any{"a", map[string]any{"text": "b", "done": true}}); isErr {
		t.Fatal(out)
	}
	after, _ := e.store.ListCriteria(bg, w.ID, is.ID)
	same(stored(), "a", "b")
	if len(before) != len(after) || before[0].ID != after[0].ID || before[1].ID != after[1].ID {
		t.Fatalf("rows churned: %v -> %v", before, after)
	}
	if !after[1].Done || after[0].Done {
		t.Fatalf("done flags wrong: %+v", after)
	}

	// A typed item with a check is stored.
	if out, isErr = set([]any{map[string]any{"text": "build", "kind": "deterministic",
		"check": map[string]any{"cmd": "true", "expect_exit": 0}}}); isErr {
		t.Fatal(out)
	}
	cs, _ := e.store.ListCriteria(bg, w.ID, is.ID)
	if len(cs) != 1 || cs[0].Kind != models.CriterionDeterministic || len(cs[0].CheckSpec) == 0 {
		t.Fatalf("typed item: %+v", cs)
	}
}
