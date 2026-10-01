package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"raenil/internal/auth"
	"raenil/internal/models"
	"raenil/internal/store"
)

// asUser runs h as u through the real auth middleware (bearer token), so the
// handler sees the user exactly as it does in production.
func asUser(t *testing.T, srv *Server, st *store.Store, u models.User, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	tok := "push-test-" + u.ID
	_, _ = st.CreateAPIToken(context.Background(), u.ID, "t", auth.HashToken(tok), nil)
	r := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	srv.auth.Middleware(h).ServeHTTP(rec, r)
	return rec
}

func TestPushSubscribeValidatesEndpoint(t *testing.T) {
	st := testStore(t)
	u := newUser(t, st)
	srv := &Server{store: st, auth: auth.NewManager(st, false)}
	srv.pushResolve = func(_ context.Context, host string) ([]net.IP, error) {
		if host == "internal.example" {
			return []net.IP{net.ParseIP("10.1.2.3")}, nil
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	sub := func(ep string) string {
		return `{"endpoint":"` + ep + `","expirationTime":null,"keys":{"p256dh":"p","auth":"a"}}`
	}

	for _, ep := range []string{
		"http://push.example/x",
		"https://127.0.0.1/x",
		"https://[::1]/x",
		"https://169.254.169.254/latest",
		"https://internal.example/x",
		"https://localhost/x",
		"not a url",
		"javascript:alert(1)",
	} {
		rec := asUser(t, srv, st, u, srv.handlePushSubscribe, sub(ep))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_endpoint") {
			t.Errorf("%q: %d %s, want 400 invalid_endpoint", ep, rec.Code, rec.Body)
		}
	}

	good := "https://push.example/send/" + u.ID
	t.Cleanup(func() { _ = st.DeletePushSubscriptionByEndpoint(context.Background(), good) })
	if rec := asUser(t, srv, st, u, srv.handlePushSubscribe, sub(good)); rec.Code != http.StatusCreated {
		t.Fatalf("valid endpoint: %d %s", rec.Code, rec.Body)
	}
}

func TestPushRebindAndUserScopedUnsubscribe(t *testing.T) {
	st := testStore(t)
	a, b := newUser(t, st), newUser(t, st)
	wsAB := newWorkspace(t, st, a)
	if err := st.AddMember(context.Background(), wsAB.ID, b.ID, models.RoleMember); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: st, auth: auth.NewManager(st, false)}
	srv.pushResolve = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	ep := "https://push.example/device/" + a.ID
	t.Cleanup(func() { _ = st.DeletePushSubscriptionByEndpoint(context.Background(), ep) })
	body := `{"endpoint":"` + ep + `","keys":{"p256dh":"p","auth":"a"}}`
	unsub := `{"endpoint":"` + ep + `"}`

	if rec := asUser(t, srv, st, a, srv.handlePushSubscribe, body); rec.Code != http.StatusCreated {
		t.Fatalf("a subscribe: %d %s", rec.Code, rec.Body)
	}
	// B cannot delete A's subscription.
	rec := asUser(t, srv, st, b, srv.handlePushUnsubscribe, unsub)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("b unsubscribes a's endpoint: %d %s, want 404", rec.Code, rec.Body)
	}
	subs, _ := st.ListPushSubscriptions(context.Background(), wsAB.ID)
	if len(subs) != 1 {
		t.Fatalf("a's subscription was deleted by b: %v", subs)
	}

	// Re-subscribing the same endpoint as B rebinds it to B.
	if rec := asUser(t, srv, st, b, srv.handlePushSubscribe, body); rec.Code != http.StatusCreated {
		t.Fatalf("b subscribe: %d %s", rec.Code, rec.Body)
	}
	var owner string
	if err := testPool.QueryRow(context.Background(), `SELECT user_id::text FROM push_subscriptions WHERE endpoint=$1`, ep).Scan(&owner); err != nil || owner != b.ID {
		t.Fatalf("owner = %q (err %v), want b %q", owner, err, b.ID)
	}
	// Now A is the stranger.
	if rec := asUser(t, srv, st, a, srv.handlePushUnsubscribe, unsub); rec.Code != http.StatusNotFound {
		t.Fatalf("a unsubscribes b's endpoint: %d, want 404", rec.Code)
	}
	// The owner can delete.
	if rec := asUser(t, srv, st, b, srv.handlePushUnsubscribe, unsub); rec.Code != http.StatusOK {
		t.Fatalf("owner unsubscribe: %d %s", rec.Code, rec.Body)
	}
	if subs, _ := st.ListPushSubscriptions(context.Background(), wsAB.ID); len(subs) != 0 {
		t.Fatalf("subscription survived owner delete: %v", subs)
	}
}
