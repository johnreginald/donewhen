package store

import (
	"context"
	"os"
	"testing"

	"raenil/internal/db"
)

// These tests need a throwaway Postgres. Set RAENIL_TEST_DATABASE_URL to run;
// otherwise they skip (so `go test ./...` stays green without a database).
func testStore(t *testing.T) *Store {
	dsn := os.Getenv("RAENIL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RAENIL_TEST_DATABASE_URL to run store tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(pool, "K")
}

func TestIssueKeyIncrements(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, err := s.CreateIssue(ctx, IssueInput{Title: "a", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateIssue(ctx, IssueInput{Title: "b", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Key == b.Key {
		t.Fatalf("keys not unique: %s == %s", a.Key, b.Key)
	}
	if b.Number <= a.Number {
		t.Fatalf("number did not increment: %d then %d", a.Number, b.Number)
	}
}

func TestExclusiveLabelGroupLastWins(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	// bug and feature are both in the exclusive 'type' group (seeded).
	is, err := s.CreateIssue(ctx, IssueInput{
		Title:      "exclusive",
		StateName:  "Backlog",
		LabelNames: []string{"bug", "feature"},
	})
	if err != nil {
		t.Fatal(err)
	}
	typeLabels := 0
	for _, l := range is.Labels {
		if l.Name == "bug" || l.Name == "feature" {
			typeLabels++
		}
	}
	if typeLabels != 1 {
		t.Fatalf("expected exactly 1 label from exclusive 'type' group, got %d (%v)", typeLabels, is.Labels)
	}
	if is.Labels[0].Name != "feature" {
		t.Fatalf("expected last-wins 'feature', got %q", is.Labels[0].Name)
	}
}

func TestStateMovePersists(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	is, err := s.CreateIssue(ctx, IssueInput{Title: "move me", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	name := "In Review"
	updated, err := s.UpdateIssue(ctx, is.ID, IssuePatch{StateName: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.StateID == is.StateID {
		t.Fatal("state did not change")
	}
	st, err := s.GetState(ctx, updated.StateID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "In Review" {
		t.Fatalf("expected In Review, got %s", st.Name)
	}
}
