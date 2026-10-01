package events

import (
	"testing"
	"time"
)

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(time.Second):
		t.Fatal("no event")
		return Event{}
	}
}

func none(t *testing.T, ch <-chan Event) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatalf("unexpected event %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscribeIsScopedToWorkspace(t *testing.T) {
	b := NewBus()
	a := b.Subscribe("A")
	defer a.Close()
	b.Publish(Event{Type: IssueCreated, WorkspaceID: "B"})
	none(t, a.Events)
	b.Publish(Event{Type: IssueCreated, WorkspaceID: "A"})
	if e := recv(t, a.Events); e.WorkspaceID != "A" {
		t.Fatalf("got %+v", e)
	}
}

// PP-196: the empty id used to mean "everything".
func TestSubscribeEmptyIsNotAWildcard(t *testing.T) {
	b := NewBus()
	s := b.Subscribe("")
	defer s.Close()
	b.Publish(Event{Type: IssueCreated, WorkspaceID: "A"})
	b.Publish(Event{Type: IssueCreated, WorkspaceID: ""})
	none(t, s.Events)
}

func TestSubscribeAllSeesEveryWorkspace(t *testing.T) {
	b := NewBus()
	s := b.SubscribeAll()
	defer s.Close()
	b.Publish(Event{Type: IssueCreated, WorkspaceID: "A"})
	b.Publish(Event{Type: IssueCreated, WorkspaceID: "B"})
	if e := recv(t, s.Events); e.WorkspaceID != "A" {
		t.Fatalf("got %+v", e)
	}
	if e := recv(t, s.Events); e.WorkspaceID != "B" {
		t.Fatalf("got %+v", e)
	}
}

func TestLaggedSubscriberGetsExactlyOneResync(t *testing.T) {
	b := NewBus()
	s := b.Subscribe("A")
	defer s.Close()
	select {
	case <-s.Resync:
		t.Fatal("resync before any drop")
	default:
	}
	// Overflow the 32-slot buffer by a lot: many drops, still one signal.
	for i := 0; i < 100; i++ {
		b.Publish(Event{Type: IssueUpdated, WorkspaceID: "A"})
	}
	select {
	case <-s.Resync:
	case <-time.After(time.Second):
		t.Fatal("no resync after overflow")
	}
	select {
	case <-s.Resync:
		t.Fatal("second resync for the same lag")
	default:
	}
	// Drain, overflow again: the flag re-arms.
	for len(s.Events) > 0 {
		<-s.Events
	}
	for i := 0; i < 40; i++ {
		b.Publish(Event{Type: IssueUpdated, WorkspaceID: "A"})
	}
	select {
	case <-s.Resync:
	default:
		t.Fatal("flag did not re-arm")
	}
}

func TestHealthySubscriberNeverResyncs(t *testing.T) {
	b := NewBus()
	s := b.Subscribe("A")
	defer s.Close()
	for i := 0; i < 100; i++ {
		b.Publish(Event{Type: IssueUpdated, WorkspaceID: "A"})
		<-s.Events
	}
	select {
	case <-s.Resync:
		t.Fatal("resync for a subscriber that kept up")
	default:
	}
}
