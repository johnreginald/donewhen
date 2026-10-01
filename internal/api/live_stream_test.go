package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
)

// openStream opens GET /api/events on a live server as the given cookies and
// returns the response lines on a channel that closes when the stream ends.
func openStream(t *testing.T, e *testEnv, ws string, cookies []*http.Cookie) <-chan string {
	t.Helper()
	srv := httptest.NewServer(e.handler)
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest("GET", srv.URL+"/api/events", nil)
	req.Header.Set("X-Workspace", ws)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("open stream: %d", resp.StatusCode)
	}
	t.Cleanup(func() { resp.Body.Close() })
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	waitStreamLine(t, ch, ": connected")
	return ch
}

func waitStreamLine(t *testing.T, ch <-chan string, want string) {
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

func streamClosesWithin(ch <-chan string, d time.Duration) bool {
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// PP-196 part 1: a caller with no workspace to resolve is refused, never given
// a wildcard stream.
func TestEventsStreamRefusesCallerWithNoWorkspace(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	cookies := e.session(uid)
	if w := e.do("GET", "/api/events", nil, browser(cookies)); w.Code != 403 {
		t.Fatalf("no workspace: %d %s, want 403", w.Code, w.Body.String())
	}
}

// PP-196 part 2: removing a member closes their open stream at once.
func TestRemovingMemberClosesTheirOpenStream(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	member, _ := e.user("")
	w := e.workspace(owner)
	if err := e.store.AddMember(context.Background(), w.ID, member, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	memberStream := openStream(t, e, w.ID, e.session(member))
	ownerCookies := e.session(owner)
	ownerStream := openStream(t, e, w.ID, ownerCookies)

	rec := e.do("DELETE", "/api/workspaces/"+w.ID+"/members/"+member, nil,
		browser(ownerCookies), header("X-Workspace", w.ID))
	if rec.Code != 200 {
		t.Fatalf("remove member: %d %s", rec.Code, rec.Body.String())
	}
	if !streamClosesWithin(memberStream, time.Second) {
		t.Fatal("removed member's stream still open after 1s")
	}

	// The owner's stream stays open and does not see the removal event.
	e.srv.bus.Publish(events.Event{Type: events.IssueCreated, WorkspaceID: w.ID, IssueID: "after-removal"})
	waitStreamLine(t, ownerStream, "after-removal")
}

// PP-196 part 2: a session that expires closes the stream by the next ping.
func TestExpiredSessionClosesStreamAtNextPing(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	w := e.workspace(uid)
	e.srv.sse.PingInterval = 50 * time.Millisecond
	cookies := e.session(uid)
	ch := openStream(t, e, w.ID, cookies)
	waitStreamLine(t, ch, ": ping") // valid session keeps the stream alive

	if _, err := e.pool.Exec(context.Background(),
		`UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if !streamClosesWithin(ch, 2*time.Second) {
		t.Fatal("stream still open after the session expired")
	}
}

// PP-196 part 2: a membership dropped behind the bus's back (no event) is
// still caught by the ping re-check.
func TestPingClosesStreamWhenMembershipVanishes(t *testing.T) {
	e := newEnv(t, config.Config{})
	owner, _ := e.user("")
	member, _ := e.user("")
	w := e.workspace(owner)
	if err := e.store.AddMember(context.Background(), w.ID, member, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	e.srv.sse.PingInterval = 50 * time.Millisecond
	ch := openStream(t, e, w.ID, e.session(member))
	if _, err := e.pool.Exec(context.Background(),
		`DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, w.ID, member); err != nil {
		t.Fatal(err)
	}
	if !streamClosesWithin(ch, 2*time.Second) {
		t.Fatal("stream still open after membership vanished")
	}
}

// Revalidate covers both credential kinds.
func TestRevalidate(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	other, _ := e.user("")
	w := e.workspace(uid)
	tok := e.token(uid, w.ID)
	cookies := e.session(uid)

	withBearer := httptest.NewRequest("GET", "/api/events", nil)
	withBearer.Header.Set("Authorization", "Bearer "+tok)
	withCookie := httptest.NewRequest("GET", "/api/events", nil)
	for _, c := range cookies {
		withCookie.AddCookie(c)
	}
	if !e.srv.auth.Revalidate(withBearer, w.ID, uid) || !e.srv.auth.Revalidate(withCookie, w.ID, uid) {
		t.Fatal("live credentials rejected")
	}
	if e.srv.auth.Revalidate(withCookie, w.ID, other) {
		t.Fatal("credential accepted for a different user")
	}
	if e.srv.auth.Revalidate(httptest.NewRequest("GET", "/api/events", nil), w.ID, uid) {
		t.Fatal("request with no credential accepted")
	}
	bad := httptest.NewRequest("GET", "/api/events", nil)
	bad.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: "nope"})
	if e.srv.auth.Revalidate(bad, w.ID, uid) {
		t.Fatal("unknown session accepted")
	}
}
