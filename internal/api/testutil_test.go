package api

import (
	"context"
	"fmt"
	"github.com/johnreginald/donewhen/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/db"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

// testStore connects to the throwaway Postgres named by DONEWHEN_TEST_DATABASE_URL
// and skips when it is unset, like the store tests.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := config.Getenv("DONEWHEN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set DONEWHEN_TEST_DATABASE_URL to run API tests")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	testPool = pool
	t.Cleanup(pool.Close)
	return store.New(pool, "K")
}

var (
	counter  atomic.Int64
	testPool *pgxpool.Pool
)

func newUser(t *testing.T, st *store.Store) models.User {
	t.Helper()
	email := fmt.Sprintf("api%d-%d@example.test", time.Now().UnixNano(), counter.Add(1))
	u, err := st.CreateUser(context.Background(), email, "x")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID) })
	return u
}

// newWorkspace makes an isolated workspace with the given user as a member.
func newWorkspace(t *testing.T, st *store.Store, u models.User) models.Workspace {
	t.Helper()
	ctx := context.Background()
	n := counter.Add(1)
	suffix := fmt.Sprintf("%d%d", time.Now().UnixNano()%100000, n)
	w, err := st.CreateWorkspace(ctx, "API "+suffix, "api-"+suffix, "A"+suffix, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, w.ID) })
	return w
}

// call runs a handler as user u acting in workspace w, bypassing the auth
// middleware (which has its own tests) but keeping everything the handler reads
// from the context.
func call(t *testing.T, h http.HandlerFunc, u models.User, w models.Workspace, method, body string, path map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/x", strings.NewReader(body))
	for k, v := range path {
		r.SetPathValue(k, v)
	}
	ctx := auth.WithWorkspace(r.Context(), w, models.RoleOwner)
	rec := httptest.NewRecorder()
	h(rec, r.WithContext(ctx))
	return rec
}
