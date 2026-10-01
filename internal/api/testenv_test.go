package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"raenil/internal/auth"
	"raenil/internal/config"
	"raenil/internal/db"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/service"
	"raenil/internal/store"
)

// These tests need a throwaway Postgres. Set RAENIL_TEST_DATABASE_URL to run;
// otherwise they skip, so `go test ./...` stays green without a database.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("RAENIL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RAENIL_TEST_DATABASE_URL to run api tests")
	}
	return dsn
}

type testEnv struct {
	t       *testing.T
	pool    *pgxpool.Pool
	store   *store.Store
	srv     *Server
	handler http.Handler
}

var seq atomic.Int64

func uniq() string { return fmt.Sprintf("%d%d", time.Now().UnixNano()%1000000, seq.Add(1)) }

// newEnv builds the real server against the test database.
func newEnv(t *testing.T, cfg config.Config) *testEnv {
	t.Helper()
	return newEnvOn(t, testDSN(t), cfg)
}

func newEnvOn(t *testing.T, dsn string, cfg config.Config) *testEnv {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := store.New(pool, "K")
	bus := events.NewBus()
	srv := NewServer(cfg, st, service.New(st, bus), bus, nil)
	return &testEnv{t: t, pool: pool, store: st, srv: srv, handler: srv.Handler()}
}

// user creates an account with a real argon2 hash of password.
func (e *testEnv) user(password string) (id, email string) {
	e.t.Helper()
	hash := "x"
	if password != "" {
		var err error
		if hash, err = auth.HashPassword(password); err != nil {
			e.t.Fatal(err)
		}
	}
	email = fmt.Sprintf("u%s@example.test", uniq())
	u, err := e.store.CreateUser(context.Background(), email, hash)
	if err != nil {
		e.t.Fatalf("create user: %v", err)
	}
	e.t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID) })
	return u.ID, email
}

// workspace creates an isolated workspace owned by owner (may be "").
func (e *testEnv) workspace(owner string) models.Workspace {
	e.t.Helper()
	n := uniq()
	w, err := e.store.CreateWorkspace(context.Background(), "Test "+n, "test-"+n, "T"+n, owner)
	if err != nil {
		e.t.Fatalf("create workspace: %v", err)
	}
	e.t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, w.ID) })
	return w
}

// token mints a bearer token for user, optionally pinned to a workspace.
func (e *testEnv) token(userID string, pin string) string {
	e.t.Helper()
	raw, _ := auth.RandomToken(32)
	plain := "raenil_" + raw
	var p *string
	if pin != "" {
		p = &pin
	}
	if _, err := e.store.CreateAPIToken(context.Background(), userID, "t", auth.HashToken(plain), p); err != nil {
		e.t.Fatalf("create token: %v", err)
	}
	return plain
}

// session returns the cookies a browser would hold after login.
func (e *testEnv) session(userID string) []*http.Cookie {
	e.t.Helper()
	c, err := e.srv.auth.StartSession(context.Background(), userID)
	if err != nil {
		e.t.Fatal(err)
	}
	csrf, _ := e.srv.auth.NewCSRFCookie()
	return []*http.Cookie{c, csrf}
}

type reqOpt func(*http.Request)

func bearer(tok string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
}

// browser attaches session cookies and the matching CSRF header.
func browser(cookies []*http.Cookie) reqOpt {
	return func(r *http.Request) {
		for _, c := range cookies {
			r.AddCookie(c)
			if c.Name == auth.CSRFCookie {
				r.Header.Set(auth.CSRFHeader, c.Value)
			}
		}
	}
}

func header(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

func remote(addr string) reqOpt { return func(r *http.Request) { r.RemoteAddr = addr } }

// do sends one request through the full handler chain.
func (e *testEnv) do(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			e.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &buf)
	r.RemoteAddr = "192.0.2.1:1234"
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, r)
	return w
}

func errBody(w *httptest.ResponseRecorder) string {
	var m map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m["error"]
}
