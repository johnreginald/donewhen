package store

import (
	"context"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-186: every issue change has its timeline row, written in the same
// transaction, or neither exists.

func activityRows(t *testing.T, s *Store, ws, issueID string) []models.Activity {
	t.Helper()
	rows, err := s.ListActivity(context.Background(), ws, issueID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func countKind(rows []models.Activity, kind string) int {
	n := 0
	for _, r := range rows {
		if r.Kind == kind {
			n++
		}
	}
	return n
}

func TestIssueChangesAreLogged(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	user := newUser(t, s)
	if err := s.AddMember(ctx, ws, user, models.RoleMember); err != nil {
		t.Fatal(err)
	}

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "log me", StateName: "Backlog", Actor: "ai"})
	if err != nil {
		t.Fatal(err)
	}
	rows := activityRows(t, s, ws, is.ID)
	if len(rows) != 1 || rows[0].Kind != "created" || rows[0].ToVal != "Backlog" || rows[0].Actor != "ai" {
		t.Fatalf("create rows = %+v", rows)
	}
	update := func(p IssuePatch) models.Issue {
		t.Helper()
		p.Actor = "human"
		out, err := s.UpdateIssue(ctx, ws, is.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	str := func(v string) *string { return &v }
	total := func() int { return len(activityRows(t, s, ws, is.ID)) }

	// Labels: one row, sorted names, comma-joined.
	before := total()
	update(IssuePatch{ReplaceLabels: true, LabelNames: []string{"bug"}})
	rows = activityRows(t, s, ws, is.ID)
	if total() != before+1 || countKind(rows, "labels_changed") != 1 {
		t.Fatalf("labels rows = %+v", rows)
	}
	if r := rows[0]; r.Kind != "labels_changed" || r.FromVal != "" || r.ToVal != "bug" || r.Actor != "human" {
		t.Fatalf("labels row = %+v", r)
	}

	// Assignee: the user's email.
	before = total()
	update(IssuePatch{SetAssignee: true, AssigneeID: str(user)})
	rows = activityRows(t, s, ws, is.ID)
	if total() != before+1 || rows[0].Kind != "assignee_changed" || !strings.HasSuffix(rows[0].ToVal, "@example.test") {
		t.Fatalf("assignee rows = %+v", rows)
	}

	// Description: a row with no body copy. Title: old and new.
	before = total()
	update(IssuePatch{DescriptionMD: str("a long new description")})
	rows = activityRows(t, s, ws, is.ID)
	if total() != before+1 || rows[0].Kind != "description_changed" || rows[0].FromVal != "" || rows[0].ToVal != "" {
		t.Fatalf("description rows = %+v", rows)
	}
	before = total()
	update(IssuePatch{Title: str("renamed")})
	rows = activityRows(t, s, ws, is.ID)
	if total() != before+1 || rows[0].Kind != "title_changed" || rows[0].FromVal != "log me" || rows[0].ToVal != "renamed" {
		t.Fatalf("title rows = %+v", rows)
	}

	// Priority and state keep their existing kinds.
	before = total()
	prio := 2
	update(IssuePatch{Priority: &prio, StateName: str("In Progress")})
	rows = activityRows(t, s, ws, is.ID)
	if total() != before+2 || countKind(rows, "priority_changed") != 1 || countKind(rows, "state_changed") != 1 {
		t.Fatalf("priority/state rows = %+v", rows)
	}

	// Saving the same values again: no rows and no updated_at bump.
	cur, _ := s.GetIssue(ctx, ws, is.ID)
	before = total()
	same := update(IssuePatch{
		Title: str("renamed"), DescriptionMD: str("a long new description"), Priority: &prio,
		StateName: str("In Progress"), SetAssignee: true, AssigneeID: str(user),
		ReplaceLabels: true, LabelNames: []string{"bug"},
	})
	if total() != before {
		t.Fatalf("no-op wrote rows: %+v", activityRows(t, s, ws, is.ID))
	}
	if !same.UpdatedAt.Equal(cur.UpdatedAt) {
		t.Fatalf("no-op bumped updated_at: %v -> %v", cur.UpdatedAt, same.UpdatedAt)
	}
}

// A failing activity insert rolls the issue change back and reaches the caller.
func TestActivityFailureRollsBackChange(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "before", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	// Make every activity insert for this workspace fail.
	fn := "fail_activity_" + strings.ReplaceAll(ws, "-", "")
	for _, q := range []string{
		`CREATE FUNCTION ` + fn + `() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF NEW.workspace_id = '` + ws + `' THEN RAISE EXCEPTION 'forced activity failure'; END IF;
			RETURN NEW; END $$`,
		`CREATE TRIGGER ` + fn + ` BEFORE INSERT ON activity FOR EACH ROW EXECUTE FUNCTION ` + fn + `()`,
	} {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(ctx, `DROP TRIGGER IF EXISTS `+fn+` ON activity`)
		_, _ = s.pool.Exec(ctx, `DROP FUNCTION IF EXISTS `+fn+`()`)
	})

	title := "after"
	if _, err := s.UpdateIssue(ctx, ws, is.ID, IssuePatch{Title: &title}); err == nil ||
		!strings.Contains(err.Error(), "forced activity failure") {
		t.Fatalf("err = %v", err)
	}
	got, err := s.GetIssue(ctx, ws, is.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "before" || !got.UpdatedAt.Equal(is.UpdatedAt) {
		t.Fatalf("issue changed despite the failed activity insert: %+v", got)
	}
	// Create rolls back too: no issue is left behind.
	if _, err := s.CreateIssue(ctx, ws, IssueInput{Title: "ghost", StateName: "Backlog"}); err == nil {
		t.Fatal("create should fail")
	}
	all, _ := s.ListIssues(ctx, IssueFilter{WorkspaceID: ws})
	if len(all) != 1 {
		t.Fatalf("issues = %d, want 1", len(all))
	}
	// Commits and criteria roll back with their row as well.
	if _, err := s.AddCommit(ctx, ws, is.ID, "abc1234", "msg", nil); err == nil {
		t.Fatal("commit link should fail")
	}
	if cs, _ := s.ListCommits(ctx, ws, is.ID); len(cs) != 0 {
		t.Fatalf("commit left behind: %+v", cs)
	}
}

// Moving to In Review is recorded as state_changed in the same transaction, so
// the inbox "Needs review" query keeps finding the issue.
func TestInReviewStillInInbox(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "review me", StateName: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceCriteria(ctx, ws, is.ID, []CriterionInput{{Body: "done", Done: true}}, "ai"); err != nil {
		t.Fatal(err)
	}
	st := "In Review"
	if _, err := s.UpdateIssue(ctx, ws, is.ID, IssuePatch{StateName: &st, Actor: "ai"}); err != nil {
		t.Fatal(err)
	}
	items, err := s.InboxNeedsReview(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Issue.ID != is.ID || items[0].EnteredReviewAt == nil {
		t.Fatalf("inbox = %+v", items)
	}
}

func TestCommitAndCriterionRows(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "records", StateName: "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCommitAs(ctx, ws, is.ID, "abc1234def", "PP-1: thing", nil, "ai"); err != nil {
		t.Fatal(err)
	}
	rows := activityRows(t, s, ws, is.ID)
	if r := rows[0]; r.Kind != "commit_linked" || r.ToVal != "abc1234def" || r.FromVal != "" || r.Actor != "ai" {
		t.Fatalf("commit row = %+v", r)
	}

	crit, err := s.ReplaceCriteria(ctx, ws, is.ID,
		[]CriterionInput{{Body: "one"}, {Body: "two", Done: true}}, "ai")
	if err != nil {
		t.Fatal(err)
	}
	// A new item that arrives ticked is one row; an unticked one is none.
	if n := countKind(activityRows(t, s, ws, is.ID), "criterion_checked"); n != 1 {
		t.Fatalf("after first set: %d rows", n)
	}

	// Tick one, then untick it: one row each, recording text and done.
	done := true
	if _, err := s.UpdateCriterionAs(ctx, ws, crit[0].ID, nil, &done, nil, nil, nil, "ai"); err != nil {
		t.Fatal(err)
	}
	rows = activityRows(t, s, ws, is.ID)
	if r := rows[0]; r.Kind != "criterion_checked" || r.FromVal != "one" || r.ToVal != "true" || r.Actor != "ai" {
		t.Fatalf("tick row = %+v", r)
	}
	// Setting done to the value it already has writes nothing.
	n := len(rows)
	if _, err := s.UpdateCriterionAs(ctx, ws, crit[0].ID, nil, &done, nil, nil, nil, "ai"); err != nil {
		t.Fatal(err)
	}
	if got := len(activityRows(t, s, ws, is.ID)); got != n {
		t.Fatalf("unchanged done wrote a row")
	}
	notDone := false
	if _, err := s.UpdateCriterionAs(ctx, ws, crit[0].ID, nil, &notDone, nil, nil, nil, "human"); err != nil {
		t.Fatal(err)
	}
	rows = activityRows(t, s, ws, is.ID)
	if r := rows[0]; r.Kind != "criterion_checked" || r.ToVal != "false" || r.Actor != "human" {
		t.Fatalf("untick row = %+v", r)
	}

	// set_criteria semantics: flip one item -> one row with its text; resend with
	// no done change -> no rows.
	n = len(rows)
	if _, err := s.ReplaceCriteria(ctx, ws, is.ID,
		[]CriterionInput{{Body: "one", Done: true}, {Body: "two", Done: true}}, "ai"); err != nil {
		t.Fatal(err)
	}
	rows = activityRows(t, s, ws, is.ID)
	if len(rows) != n+1 || rows[0].Kind != "criterion_checked" || rows[0].FromVal != "one" || rows[0].ToVal != "true" {
		t.Fatalf("flip rows = %+v", rows[:2])
	}
	if _, err := s.ReplaceCriteria(ctx, ws, is.ID,
		[]CriterionInput{{Body: "one", Done: true}, {Body: "two", Done: true}}, "ai"); err != nil {
		t.Fatal(err)
	}
	if got := len(activityRows(t, s, ws, is.ID)); got != n+1 {
		t.Fatalf("unchanged resend wrote rows: %d", got-n-1)
	}
}
