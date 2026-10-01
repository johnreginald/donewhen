package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// PP-187: linking the same commit twice keeps one row (and one timeline row);
// the sha is stored lowercase.
func TestAddCommitIsIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "commits", StateName: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.AddCommit(ctx, ws, is.ID, "ABCDEF1234", "first message", nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA != "abcdef1234" {
		t.Fatalf("sha stored as %q", first.SHA)
	}
	again, err := s.AddCommit(ctx, ws, is.ID, "abcdef1234", "second message", nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.Message != "first message" {
		t.Fatalf("second link did not return the existing row: %+v", again)
	}
	if cs, _ := s.ListCommits(ctx, ws, is.ID); len(cs) != 1 {
		t.Fatalf("commits = %d, want 1", len(cs))
	}
	if n := countKind(activityRows(t, s, ws, is.ID), "commit_linked"); n != 1 {
		t.Fatalf("commit_linked rows = %d, want 1", n)
	}
	// Another issue may link the same sha; an unknown issue is not found.
	other, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "other", StateName: "Backlog"})
	if _, err := s.AddCommit(ctx, ws, other.ID, "abcdef1234", "", nil); err != nil {
		t.Fatalf("same sha on another issue: %v", err)
	}
	if _, err := s.AddCommit(ctx, ws, "00000000-0000-0000-0000-000000000000", "abcdef1234", "", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
	// A malformed sha is refused before anything is written.
	for _, bad := range []string{"", "abc", "xyz1234", "abc%def"} {
		if _, err := s.AddCommit(ctx, ws, is.ID, bad, "", nil); !errors.Is(err, ErrInvalid) {
			t.Errorf("AddCommit %q: %v", bad, err)
		}
	}
}

func TestIssueByCommitStrictPrefix(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "lookup", StateName: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	full := "0123456789abcdef0123456789abcdef01234567"
	if _, err := s.AddCommit(ctx, ws, is.ID, full, "m", nil); err != nil {
		t.Fatal(err)
	}
	wss := []string{ws}

	// Malformed input is an error naming the problem.
	for in, want := range map[string]string{
		"":         "sha required",
		"   ":      "sha required",
		"a":        "at least 7",
		"abcdef":   "at least 7",
		"abc%def":  "must be hex",
		"abc_def1": "must be hex",
		"0123456789abcdef0123456789abcdef012345678": "at most 40",
	} {
		if _, err := s.IssueByCommit(ctx, wss, in); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("IssueByCommit(%q) = %v, want invalid containing %q", in, err, want)
		}
	}
	// A 7-char prefix, the full sha, and an uppercase prefix all find the issue.
	for _, q := range []string{full[:7], full, strings.ToUpper(full[:12])} {
		got, err := s.IssueByCommit(ctx, wss, q)
		if err != nil || got.IssueID != is.ID || got.SHA != full {
			t.Errorf("IssueByCommit(%q) = %+v, %v", q, got, err)
		}
	}
	// A sha that is not a prefix of anything: not found.
	if _, err := s.IssueByCommit(ctx, wss, "fedcba9"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no match: %v", err)
	}
	// No reverse match: a stored short sha does not answer for a longer query.
	short, _ := s.CreateIssue(ctx, ws, IssueInput{Title: "short", StateName: "Backlog"})
	if _, err := s.AddCommit(ctx, ws, short.ID, "aaaaaaa", "m", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueByCommit(ctx, wss, "aaaaaaa1234567"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reverse match: %v", err)
	}
	// Scoped to the given workspaces.
	if _, err := s.IssueByCommit(ctx, []string{newWorkspace(t, s)}, full[:7]); !errors.Is(err, ErrNotFound) {
		t.Errorf("other workspace: %v", err)
	}
}
