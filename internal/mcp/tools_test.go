package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/server"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/db"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/service"
	"github.com/johnreginald/donewhen/internal/store"
)

// These tests need a throwaway Postgres. Set DONEWHEN_TEST_DATABASE_URL to run;
// otherwise they skip, so `go test ./...` stays green without a database.
type testEnv struct {
	t     *testing.T
	pool  *pgxpool.Pool
	store *store.Store
	mgr   *auth.Manager
	srv   *server.MCPServer
}

var seq atomic.Int64

func uniq() string { return fmt.Sprintf("%d%d", time.Now().UnixNano()%1000000, seq.Add(1)) }

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	dsn := config.Getenv("DONEWHEN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DONEWHEN_TEST_DATABASE_URL to run mcp tool tests")
	}
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
	mgr := auth.NewManager(st, false)
	d := &deps{svc: service.New(st, events.NewBus()), store: st, cfg: config.Config{}, mgr: mgr}
	return &testEnv{t: t, pool: pool, store: st, mgr: mgr, srv: buildServer(d)}
}

func (e *testEnv) user() string {
	e.t.Helper()
	u, err := e.store.CreateUser(context.Background(), fmt.Sprintf("m%s@example.test", uniq()), "x")
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID) })
	return u.ID
}

func (e *testEnv) workspace(owner string) models.Workspace {
	e.t.Helper()
	n := uniq()
	w, err := e.store.CreateWorkspace(context.Background(), "Test "+n, "test-"+n, "T"+n, owner)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, w.ID) })
	return w
}

func (e *testEnv) member(ws, user, role string) {
	e.t.Helper()
	if err := e.store.AddMember(context.Background(), ws, user, role); err != nil {
		e.t.Fatal(err)
	}
}

// ctxFor returns the context the real auth middleware builds for a bearer
// token (user, actor "ai", and the pin), exactly as /mcp tool calls see it.
func (e *testEnv) ctxFor(userID, pin string) context.Context {
	e.t.Helper()
	raw, _ := auth.RandomToken(32)
	plain := "donewhen_" + raw
	var p *string
	if pin != "" {
		p = &pin
	}
	if _, err := e.store.CreateAPIToken(context.Background(), userID, "t", auth.HashToken(plain), p); err != nil {
		e.t.Fatal(err)
	}
	var got context.Context
	h := e.mgr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Context() }))
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Authorization", "Bearer "+plain)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got == nil {
		e.t.Fatal("middleware did not run")
	}
	if _, ok := auth.UserFrom(got); !ok {
		e.t.Fatal("token did not authenticate")
	}
	return got
}

// call runs one tool through the server and returns (text, isError).
func (e *testEnv) call(ctx context.Context, tool string, args map[string]any) (string, bool) {
	e.t.Helper()
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	resp := e.srv.HandleMessage(ctx, msg)
	b, err := json.Marshal(resp)
	if err != nil {
		e.t.Fatal(err)
	}
	var out struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		e.t.Fatalf("decode %s: %v", b, err)
	}
	if out.Error != nil {
		return out.Error.Message, true
	}
	text := ""
	if len(out.Result.Content) > 0 {
		text = out.Result.Content[0].Text
	}
	return text, out.Result.IsError
}

func (e *testEnv) wsName(id string) string {
	e.t.Helper()
	w, err := e.store.GetWorkspace(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return w.Name
}
