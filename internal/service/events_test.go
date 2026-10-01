package service

import (
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

// PP-201: every mutation publishes the right event for its workspace and
// writes the matching activity rows.

func (e *gateEnv) next(sub events.Subscription) events.Event {
	e.t.Helper()
	select {
	case ev := <-sub.Events:
		return ev
	case <-time.After(time.Second):
		e.t.Fatal("no event published")
		return events.Event{}
	}
}

func (e *gateEnv) noEvent(sub events.Subscription) {
	e.t.Helper()
	select {
	case ev := <-sub.Events:
		e.t.Fatalf("unexpected event %s", ev.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func (e *gateEnv) activity(is models.Issue) []models.Activity {
	e.t.Helper()
	rows, err := e.svc.Store.ListActivity(e.ctx, e.ws, is.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	return rows
}

func kinds(rows []models.Activity) map[string]models.Activity {
	m := map[string]models.Activity{}
	for _, r := range rows {
		m[r.Kind] = r
	}
	return m
}

func TestCreatePublishesCreatedAndLogsActivity(t *testing.T) {
	e := newGateEnv(t)
	sub := e.svc.Bus.Subscribe(e.ws)
	defer sub.Close()
	other := e.svc.Bus.Subscribe("some-other-workspace")
	defer other.Close()

	is := e.issue("Backlog")
	ev := e.next(sub)
	if ev.Type != events.IssueCreated || ev.WorkspaceID != e.ws || ev.Actor != "human" || ev.Issue == nil || ev.Issue.ID != is.ID {
		t.Fatalf("event = %+v", ev)
	}
	if ev.To == nil || ev.To.Name != "Backlog" {
		t.Fatalf("created event state = %+v", ev.To)
	}
	e.noEvent(other)

	rows := e.activity(is)
	if len(rows) != 1 || rows[0].Kind != "created" || rows[0].ToVal != "Backlog" || rows[0].Actor != "human" {
		t.Fatalf("activity = %+v", rows)
	}
}

func TestStateChangePublishesStateChangedNotUpdated(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("Backlog")
	sub := e.svc.Bus.Subscribe(e.ws)
	defer sub.Close()

	if err := e.move(is, "Ready", "ai", false); err != nil {
		t.Fatal(err)
	}
	ev := e.next(sub)
	if ev.Type != events.IssueStateChanged || ev.Actor != "ai" {
		t.Fatalf("event = %+v, want issue.state_changed by ai", ev)
	}
	if ev.From == nil || ev.From.Name != "Backlog" || ev.To == nil || ev.To.Name != "Ready" {
		t.Fatalf("from/to = %+v / %+v", ev.From, ev.To)
	}
	e.noEvent(sub) // one event, not also an issue.updated

	row, ok := kinds(e.activity(is))["state_changed"]
	if !ok || row.Field != "status" || row.FromVal != "Backlog" || row.ToVal != "Ready" || row.Actor != "ai" {
		t.Fatalf("state_changed row = %+v (found %v)", row, ok)
	}
}

func TestFieldChangePublishesUpdatedWithMatchingActivity(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("Backlog")
	sub := e.svc.Bus.Subscribe(e.ws)
	defer sub.Close()

	title := "renamed"
	prio := 1
	if _, err := e.svc.UpdateIssue(e.ctx, e.ws, is.ID, store.IssuePatch{Title: &title, Priority: &prio}, "human"); err != nil {
		t.Fatal(err)
	}
	ev := e.next(sub)
	if ev.Type != events.IssueUpdated || ev.Issue == nil || ev.Issue.Title != "renamed" {
		t.Fatalf("event = %+v, want issue.updated", ev)
	}
	e.noEvent(sub)

	got := kinds(e.activity(is))
	if r, ok := got["title_changed"]; !ok || r.FromVal != "t" || r.ToVal != "renamed" {
		t.Fatalf("title_changed = %+v (found %v)", r, ok)
	}
	if r, ok := got["priority_changed"]; !ok || r.FromVal != "No priority" || r.ToVal != "Urgent" {
		t.Fatalf("priority_changed = %+v (found %v)", r, ok)
	}
	if _, ok := got["state_changed"]; ok {
		t.Fatal("state_changed logged for a move that did not happen")
	}
}

func TestNoOpUpdateWritesNoActivity(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("Backlog")
	before := len(e.activity(is))

	same := is.Title
	if _, err := e.svc.UpdateIssue(e.ctx, e.ws, is.ID, store.IssuePatch{Title: &same}, "human"); err != nil {
		t.Fatal(err)
	}
	if after := len(e.activity(is)); after != before {
		t.Fatalf("activity rows %d -> %d for a no-op update", before, after)
	}
}

func TestRefusedMovePublishesNothing(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("In Progress")
	e.crit(is.ID, "open item", models.CriterionManual, false)
	sub := e.svc.Bus.Subscribe(e.ws)
	defer sub.Close()
	before := len(e.activity(is))

	if err := e.move(is, "Done", "ai", false); err == nil {
		t.Fatal("gate did not refuse")
	}
	e.noEvent(sub)
	if after := len(e.activity(is)); after != before {
		t.Fatalf("a refused move logged activity: %d -> %d", before, after)
	}
}

func TestDeleteAndCommentPublishAndLog(t *testing.T) {
	e := newGateEnv(t)
	is := e.issue("Backlog")
	sub := e.svc.Bus.Subscribe(e.ws)
	defer sub.Close()

	if _, err := e.svc.AddComment(e.ctx, e.ws, is.ID, "looks good", "ai"); err != nil {
		t.Fatal(err)
	}
	ev := e.next(sub)
	if ev.Type != events.CommentAdded || ev.Comment == nil || ev.Comment.BodyMD != "looks good" || ev.Actor != "ai" {
		t.Fatalf("comment event = %+v", ev)
	}
	if r, ok := kinds(e.activity(is))["commented"]; !ok || r.Detail != "looks good" || r.Actor != "ai" {
		t.Fatalf("commented row = %+v (found %v)", r, ok)
	}

	if err := e.svc.DeleteIssue(e.ctx, e.ws, is.ID, "human"); err != nil {
		t.Fatal(err)
	}
	ev = e.next(sub)
	if ev.Type != events.IssueDeleted || ev.IssueID != is.ID || ev.WorkspaceID != e.ws {
		t.Fatalf("delete event = %+v", ev)
	}
	// The issue is gone, but the workspace's log keeps the deletion.
	rows, err := e.svc.Store.ListRecentActivity(e.ctx, store.ActivityFilter{WorkspaceID: e.ws, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := kinds(rows)["deleted"]; !ok || r.IssueKey != is.Key {
		t.Fatalf("deleted row = %+v (found %v)", r, ok)
	}
}

// A mutation in one workspace never reaches another workspace's subscriber.
func TestEventsStayInTheirWorkspace(t *testing.T) {
	a := newGateEnv(t)
	b := newGateEnv(t)
	// Share one bus so a cross-delivery bug would be visible.
	b.svc.Bus = a.svc.Bus
	subA := a.svc.Bus.Subscribe(a.ws)
	defer subA.Close()
	subB := a.svc.Bus.Subscribe(b.ws)
	defer subB.Close()

	b.issue("Backlog")
	if ev := b.next(subB); ev.WorkspaceID != b.ws {
		t.Fatalf("event for %q delivered to B", ev.WorkspaceID)
	}
	a.noEvent(subA)
}
