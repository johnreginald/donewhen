package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-184: ReplaceCriteria is atomic. A write that fails part-way leaves the
// stored list exactly as it was, and a failed read is an error, not "empty".
func TestReplaceCriteriaAtomic(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	issue, err := s.CreateIssue(ctx, ws, IssueInput{Title: "replace", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceCriteria(ctx, ws, issue.ID, []CriterionInput{{Body: "a"}, {Body: "b"}, {Body: "c"}}); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		cs, err := s.ListCriteria(ctx, ws, issue.ID)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(cs)
		return string(b)
	}
	orig := snapshot()

	// Item 2 violates the DB check constraint (typed, no spec): rolled back.
	_, err = s.ReplaceCriteria(ctx, ws, issue.ID, []CriterionInput{
		{Body: "x", Done: true}, {Body: "y", Kind: models.CriterionDeterministic},
	})
	if err == nil {
		t.Fatal("expected the second item to fail")
	}
	if got := snapshot(); got != orig {
		t.Fatalf("list changed after failed replace:\n%s\n%s", orig, got)
	}

	// A failed read must not look like an empty list.
	dead, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ReplaceCriteria(dead, ws, issue.ID, []CriterionInput{{Body: "z"}}); err == nil {
		t.Fatal("expected an error on a dead context")
	}
	if got := snapshot(); got != orig {
		t.Fatal("list changed after failed read")
	}

	// Another workspace cannot replace it.
	other := newWorkspace(t, s)
	if _, err := s.ReplaceCriteria(ctx, other, issue.ID, []CriterionInput{{Body: "z"}}); err != ErrNotFound {
		t.Fatalf("cross-workspace: %v", err)
	}
}
