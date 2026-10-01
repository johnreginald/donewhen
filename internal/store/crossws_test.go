package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"raenil/internal/models"
)

func wantInvalid(t *testing.T, err error, code string) {
	t.Helper()
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), code) {
		t.Fatalf("err = %v, want ErrInvalid containing %q", err, code)
	}
}

func countIssues(t *testing.T, s *Store, wsID string) int {
	t.Helper()
	list, err := s.ListIssues(context.Background(), IssueFilter{WorkspaceID: wsID})
	if err != nil {
		t.Fatal(err)
	}
	return len(list)
}

func TestIssueWritesRejectForeignReferences(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)

	projB, err := s.SaveProject(ctx, wsB, models.Project{Name: "B epic"})
	if err != nil {
		t.Fatal(err)
	}
	projA, err := s.SaveProject(ctx, wsA, models.Project{Name: "A epic"})
	if err != nil {
		t.Fatal(err)
	}
	userB := newUser(t, s)
	if err := s.AddMember(ctx, wsB, userB, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	userA := newUser(t, s)
	if err := s.AddMember(ctx, wsA, userA, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	issueB, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "in B", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}

	// Create.
	_, err = s.CreateIssue(ctx, wsA, IssueInput{Title: "x", StateName: "Backlog", ProjectID: &projB.ID})
	wantInvalid(t, err, "invalid_project")
	_, err = s.CreateIssue(ctx, wsA, IssueInput{Title: "x", StateName: "Backlog", AssigneeID: &userB})
	wantInvalid(t, err, "invalid_assignee")
	_, err = s.CreateIssue(ctx, wsA, IssueInput{Title: "x", StateName: "Backlog", ParentKey: &issueB.Key})
	wantInvalid(t, err, "invalid_parent")
	notUUID := "nope"
	_, err = s.CreateIssue(ctx, wsA, IssueInput{Title: "x", StateName: "Backlog", ProjectID: &notUUID})
	wantInvalid(t, err, "invalid_project")

	// A rejected create leaves nothing behind.
	if n := countIssues(t, s, wsA); n != 0 {
		t.Fatalf("rejected creates left %d issues", n)
	}

	// Legitimate references still work.
	ok, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "ok", StateName: "Backlog", ProjectID: &projA.ID, AssigneeID: &userA})
	if err != nil {
		t.Fatalf("same-workspace refs rejected: %v", err)
	}
	child, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "child", StateName: "Backlog", ParentKey: &ok.Key})
	if err != nil {
		t.Fatalf("same-workspace parent rejected: %v", err)
	}

	// Update.
	_, err = s.UpdateIssue(ctx, wsA, ok.ID, IssuePatch{SetProject: true, ProjectID: &projB.ID})
	wantInvalid(t, err, "invalid_project")
	_, err = s.UpdateIssue(ctx, wsA, ok.ID, IssuePatch{SetAssignee: true, AssigneeID: &userB})
	wantInvalid(t, err, "invalid_assignee")
	_, err = s.UpdateIssue(ctx, wsA, ok.ID, IssuePatch{SetParent: true, ParentKey: &issueB.Key})
	wantInvalid(t, err, "invalid_parent")

	// Clearing still works.
	if _, err := s.UpdateIssue(ctx, wsA, child.ID, IssuePatch{SetParent: true, SetProject: true, SetAssignee: true}); err != nil {
		t.Fatalf("clearing refs failed: %v", err)
	}
}

func TestImportRejectsForeignParent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)
	issueB, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "in B", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	k := strings.ReplaceAll(issueB.Key, "-", "")
	_, err = s.Import(ctx, wsA, ImportData{Issues: []ImportIssue{
		{ID: "IM" + k + "-9", Title: "child", ParentID: issueB.Key},
	}})
	wantInvalid(t, err, "invalid_parent")
	if n := countIssues(t, s, wsA); n != 0 {
		t.Fatalf("failed import left %d issues", n)
	}

	// A parent inside the same file is fine, whatever the order.
	_, err = s.Import(ctx, wsA, ImportData{Issues: []ImportIssue{
		{ID: "IM" + k + "-2", Title: "child", ParentID: "IM" + k + "-1"},
		{ID: "IM" + k + "-1", Title: "parent"},
	}})
	if err != nil {
		t.Fatalf("in-file parent rejected: %v", err)
	}
}

func TestSetBlockersScopedAndSerialised(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)
	a1, _ := s.CreateIssue(ctx, wsA, IssueInput{Title: "a1", StateName: "Backlog"})
	a2, _ := s.CreateIssue(ctx, wsA, IssueInput{Title: "a2", StateName: "Backlog"})
	b1, err := s.CreateIssue(ctx, wsB, IssueInput{Title: "b1", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}

	// Foreign blocker.
	_, err = s.SetBlockers(ctx, wsA, a1.ID, []string{b1.Key})
	wantInvalid(t, err, "invalid_blocker")
	_, err = s.SetBlockers(ctx, wsA, a1.ID, []string{b1.ID})
	wantInvalid(t, err, "invalid_blocker")

	// Missing and foreign target issue -> ErrNotFound (404), not an FK error.
	if _, err := s.SetBlockers(ctx, wsA, "00000000-0000-0000-0000-000000000000", []string{a1.Key}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing issue: err = %v, want ErrNotFound", err)
	}
	if _, err := s.SetBlockers(ctx, wsA, b1.ID, []string{a1.Key}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign issue: err = %v, want ErrNotFound", err)
	}

	// Concurrent A->B and B->A: exactly one wins, the other is a cycle.
	for round := 0; round < 10; round++ {
		if _, err := s.SetBlockers(ctx, wsA, a1.ID, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SetBlockers(ctx, wsA, a2.ID, nil); err != nil {
			t.Fatal(err)
		}
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, errs[0] = s.SetBlockers(ctx, wsA, a1.ID, []string{a2.Key}) }()
		go func() { defer wg.Done(); _, errs[1] = s.SetBlockers(ctx, wsA, a2.ID, []string{a1.Key}) }()
		wg.Wait()
		fails := 0
		for _, e := range errs {
			if e != nil {
				wantInvalid(t, e, "block both forever")
				fails++
			}
		}
		if fails != 1 {
			t.Fatalf("round %d: %d requests rejected, want exactly 1 (errs=%v)", round, fails, errs)
		}
	}
}

func TestIssueRepoIgnoresForeignProject(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	wsA, wsB := newWorkspace(t, s), newWorkspace(t, s)
	repo := "https://github.com/b/b"
	projB, err := s.SaveProject(ctx, wsB, models.Project{Name: "B", RepoURL: &repo})
	if err != nil {
		t.Fatal(err)
	}
	is, err := s.CreateIssue(ctx, wsA, IssueInput{Title: "a", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate legacy bad data: point the issue at B's epic directly.
	if _, err := s.pool.Exec(ctx, `UPDATE issues SET project_id=$1 WHERE id=$2`, projB.ID, is.ID); err != nil {
		t.Fatal(err)
	}
	if got := s.IssueRepo(ctx, wsA, is.ID); got != "" {
		t.Fatalf("IssueRepo leaked foreign repo %q", got)
	}
}
