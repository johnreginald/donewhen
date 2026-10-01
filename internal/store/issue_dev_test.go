package store

import (
	"context"
	"errors"
	"testing"
)

// PP-183: SetIssueDev changes only the fields it is given.
func TestSetIssueDevKeepsTheOtherField(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	str := func(v string) *string { return &v }

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "dev", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.SetIssueDev(ctx, ws, is.ID, str("feat/x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitBranch == nil || *got.GitBranch != "feat/x" || got.PRURL != nil {
		t.Fatalf("branch only: %+v", got)
	}
	// PR only: branch stays.
	got, err = s.SetIssueDev(ctx, ws, is.ID, nil, str("https://github.com/x/y/pull/1"))
	if err != nil {
		t.Fatal(err)
	}
	if got.GitBranch == nil || *got.GitBranch != "feat/x" || got.PRURL == nil {
		t.Fatalf("pr only wiped the branch: %+v", got)
	}
	// Branch only: PR stays.
	got, err = s.SetIssueDev(ctx, ws, is.ID, str("feat/y"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitBranch == nil || *got.GitBranch != "feat/y" || got.PRURL == nil {
		t.Fatalf("branch only wiped the PR: %+v", got)
	}
	// Empty string clears only that column.
	got, err = s.SetIssueDev(ctx, ws, is.ID, str(""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitBranch != nil || got.PRURL == nil {
		t.Fatalf("clear branch: %+v", got)
	}
	got, err = s.SetIssueDev(ctx, ws, is.ID, nil, str(""))
	if err != nil {
		t.Fatal(err)
	}
	if got.PRURL != nil {
		t.Fatalf("clear pr: %+v", got)
	}
	// Neither: an error, nothing written.
	if _, err := s.SetIssueDev(ctx, ws, is.ID, nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("neither: %v", err)
	}
	if _, err := s.SetIssueDev(ctx, ws, "00000000-0000-0000-0000-000000000000", str("x"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
}
