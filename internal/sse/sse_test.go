package sse

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
)

// serve runs the handler on a real server, injecting the workspace and user
// that the API's guards would have put on the context.
func serve(t *testing.T, h *Handler, ws *models.Workspace) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if ws != nil {
			ctx = auth.WithWorkspace(ctx, *ws, "member")
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// lines streams the response body line by line on a channel.
func lines(t *testing.T, srv *httptest.Server) <-chan string {
	t.Helper()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	return ch
}

func waitLine(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case l, ok := <-ch:
			if !ok {
				t.Fatalf("stream closed before %q", want)
			}
			if strings.Contains(l, want) {
				return
			}
		case <-deadline:
			t.Fatalf("no %q line", want)
		}
	}
}

func waitClosed(t *testing.T, ch <-chan string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream still open")
		}
	}
}

func TestNoWorkspaceIs403(t *testing.T) {
	bus := events.NewBus()
	srv := serve(t, NewHandler(bus, nil), nil)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "event-stream") {
		t.Fatalf("content type %q", ct)
	}
}

func TestStreamsOnlyItsWorkspace(t *testing.T) {
	bus := events.NewBus()
	srv := serve(t, NewHandler(bus, nil), &models.Workspace{ID: "A"})
	ch := lines(t, srv)
	waitLine(t, ch, ": connected")
	bus.Publish(events.Event{Type: events.IssueCreated, WorkspaceID: "B", IssueID: "other"})
	bus.Publish(events.Event{Type: events.IssueCreated, WorkspaceID: "A", IssueID: "mine"})
	waitLine(t, ch, "mine")
}

// The user is unauthenticated in these tests, so its id is "": removals name
// "" for "this client" and "someone-else" for another user.
func TestMemberRemovedClosesThatUsersStream(t *testing.T) {
	bus := events.NewBus()
	srv := serve(t, NewHandler(bus, nil), &models.Workspace{ID: "A"})
	ch := lines(t, srv)
	waitLine(t, ch, ": connected")

	// Someone else being removed does not close it, and is not forwarded.
	bus.Publish(events.Event{Type: events.MemberRemoved, WorkspaceID: "A", UserID: "someone-else"})
	bus.Publish(events.Event{Type: events.IssueCreated, WorkspaceID: "A", IssueID: "still-open"})
	waitLine(t, ch, "still-open")

	bus.Publish(events.Event{Type: events.MemberRemoved, WorkspaceID: "A", UserID: ""})
	waitClosed(t, ch, time.Second)
}

func TestMemberRemovedInOtherWorkspaceKeepsStream(t *testing.T) {
	bus := events.NewBus()
	srv := serve(t, NewHandler(bus, nil), &models.Workspace{ID: "A"})
	ch := lines(t, srv)
	waitLine(t, ch, ": connected")
	bus.Publish(events.Event{Type: events.MemberRemoved, WorkspaceID: "B", UserID: ""})
	bus.Publish(events.Event{Type: events.IssueCreated, WorkspaceID: "A", IssueID: "still-open"})
	waitLine(t, ch, "still-open")
}

func TestPingClosesStreamWhenRevalidationFails(t *testing.T) {
	bus := events.NewBus()
	var alive atomic.Bool
	alive.Store(true)
	h := NewHandler(bus, func(r *http.Request, wsID, userID string) bool {
		if wsID != "A" {
			t.Errorf("revalidate workspace %q", wsID)
		}
		return alive.Load()
	})
	h.PingInterval = 20 * time.Millisecond
	srv := serve(t, h, &models.Workspace{ID: "A"})
	ch := lines(t, srv)
	waitLine(t, ch, ": ping") // still valid: keeps pinging
	alive.Store(false)
	waitClosed(t, ch, time.Second)
}

func TestLaggedClientGetsResync(t *testing.T) {
	bus := events.NewBus()
	srv := serve(t, NewHandler(bus, nil), &models.Workspace{ID: "A"})
	ch := lines(t, srv)
	waitLine(t, ch, ": connected")

	// Publish far faster than the handler can write, so the buffer overflows.
	deadline := time.Now().Add(3 * time.Second)
	resyncs := 0
	for resyncs == 0 && time.Now().Before(deadline) {
		for i := 0; i < 5000; i++ {
			bus.Publish(events.Event{Type: events.IssueUpdated, WorkspaceID: "A"})
		}
		time.Sleep(20 * time.Millisecond)
		for len(ch) > 0 {
			if strings.Contains(<-ch, "event: resync") {
				resyncs++
			}
		}
	}
	if resyncs == 0 {
		t.Fatal("no resync event after overflow")
	}
}
