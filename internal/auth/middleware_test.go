package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/johnreginald/donewhen/internal/db"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
	"github.com/johnreginald/donewhen/internal/testdb"
)

// PP-201: workspace resolution order and the token pin, against a real
// Postgres.

var seq atomic.Int64

type env struct {
	t    *testing.T
	pool *pgxpool.Pool
	st   *store.Store
	mgr  *Manager
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := testdb.DSN(t)
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
	return &env{t: t, pool: pool, st: st, mgr: NewManager(st, false)}
}

func uniq() string { return fmt.Sprintf("%d%d", time.Now().UnixNano()%1000000, seq.Add(1)) }

func (e *env) user() models.User {
	e.t.Helper()
	u, err := e.st.CreateUser(context.Background(), fmt.Sprintf("a%s@example.test", uniq()), "x")
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID) })
	return u
}

func (e *env) workspace(owner string) models.Workspace {
	e.t.Helper()
	n := uniq()
	w, err := e.st.CreateWorkspace(context.Background(), "Auth "+n, "auth-"+n, "U"+n, owner)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id=$1`, w.ID) })
	return w
}

// token mints a bearer token and returns the plaintext.
func (e *env) token(userID, pin string) string {
	e.t.Helper()
	raw, _ := RandomToken(32)
	plain := TokenPrefix + raw
	var p *string
	if pin != "" {
		p = &pin
	}
	if _, err := e.st.CreateAPIToken(context.Background(), userID, "t", HashToken(plain), p); err != nil {
		e.t.Fatal(err)
	}
	return plain
}

// through runs a request through Middleware and returns the context the next
// handler saw.
func (e *env) through(mutate func(*http.Request)) context.Context {
	e.t.Helper()
	var got context.Context
	h := e.mgr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Context() }))
	r := httptest.NewRequest("GET", "/x", nil)
	mutate(r)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got == nil {
		e.t.Fatal("middleware did not call next")
	}
	return got
}

func (e *env) bearerCtx(tok string) context.Context {
	return e.through(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) })
}

func (e *env) sessionCtx(userID string) context.Context {
	e.t.Helper()
	c, err := e.mgr.StartSession(context.Background(), userID)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.through(func(r *http.Request) { r.AddCookie(c) })
}

func TestMiddlewareResolvesCaller(t *testing.T) {
	e := newEnv(t)
	u := e.user()
	a := e.workspace(u.ID)

	t.Run("pinned bearer is an AI actor carrying its pin", func(t *testing.T) {
		ctx := e.bearerCtx(e.token(u.ID, a.ID))
		if got, ok := UserFrom(ctx); !ok || got.ID != u.ID {
			t.Fatalf("user = %v %v", got, ok)
		}
		if ActorFrom(ctx) != ActorAI || !IsBearer(ctx) {
			t.Fatalf("actor = %q", ActorFrom(ctx))
		}
		if pin, ok := TokenPinFrom(ctx); !ok || pin != a.ID {
			t.Fatalf("pin = %q %v", pin, ok)
		}
	})
	t.Run("unpinned bearer has no pin", func(t *testing.T) {
		ctx := e.bearerCtx(e.token(u.ID, ""))
		if _, ok := TokenPinFrom(ctx); ok {
			t.Fatal("unpinned token reports a pin")
		}
		if !IsBearer(ctx) {
			t.Fatal("bearer not recognised")
		}
	})
	t.Run("session is a human actor with no pin", func(t *testing.T) {
		ctx := e.sessionCtx(u.ID)
		if got, ok := UserFrom(ctx); !ok || got.ID != u.ID {
			t.Fatalf("user = %v %v", got, ok)
		}
		if ActorFrom(ctx) != ActorHuman || IsBearer(ctx) {
			t.Fatalf("actor = %q", ActorFrom(ctx))
		}
		if _, ok := TokenPinFrom(ctx); ok {
			t.Fatal("session reports a pin")
		}
	})
	t.Run("unknown credentials leave the request anonymous", func(t *testing.T) {
		for name, mutate := range map[string]func(*http.Request){
			"bad bearer":  func(r *http.Request) { r.Header.Set("Authorization", "Bearer donewhen_nope") },
			"empty":       func(r *http.Request) { r.Header.Set("Authorization", "Bearer ") },
			"basic auth":  func(r *http.Request) { r.Header.Set("Authorization", "Basic abc") },
			"bad session": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: SessionCookie, Value: "nope"}) },
			"nothing":     func(r *http.Request) {},
		} {
			if _, ok := UserFrom(e.through(mutate)); ok {
				t.Fatalf("%s authenticated", name)
			}
		}
	})
	t.Run("RequireAuth answers 401 without a user", func(t *testing.T) {
		h := e.mgr.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("= %d", w.Code)
		}
	})
}

// resolve returns the workspace id ResolveWorkspace picks, or the error.
func (e *env) resolve(ctx context.Context, u models.User, requested string) (string, string, error) {
	e.t.Helper()
	ws, role, err := e.mgr.ResolveWorkspace(ctx, u, requested)
	return ws.ID, role, err
}

func TestResolveWorkspaceOrder(t *testing.T) {
	e := newEnv(t)
	u := e.user()
	a := e.workspace(u.ID)
	b := e.workspace(u.ID)
	c := e.workspace(u.ID)
	if err := e.st.SetLastWorkspace(context.Background(), u.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	human := e.sessionCtx(u.ID)
	pinnedB := e.bearerCtx(e.token(u.ID, b.ID))
	unpinned := e.bearerCtx(e.token(u.ID, ""))

	// 1. An explicit request wins over the pin's absence and over last-active.
	if id, role, err := e.resolve(human, u, a.Slug); err != nil || id != a.ID || role != models.RoleOwner {
		t.Fatalf("explicit by slug = %q %q %v, want %q owner", id, role, err, a.ID)
	}
	if id, _, err := e.resolve(unpinned, u, b.KeyPrefix); err != nil || id != b.ID {
		t.Fatalf("explicit by prefix = %q %v, want %q", id, err, b.ID)
	}
	// ... and a pinned token may name its own workspace.
	if id, _, err := e.resolve(pinnedB, u, b.ID); err != nil || id != b.ID {
		t.Fatalf("pinned naming its pin = %q %v", id, err)
	}

	// 2. The pin beats last-active and sole/first membership.
	if id, _, err := e.resolve(pinnedB, u, ""); err != nil || id != b.ID {
		t.Fatalf("pin = %q %v, want %q (last active is %q)", id, err, b.ID, c.ID)
	}

	// 3. Last active, for a human only.
	if id, _, err := e.resolve(human, u, ""); err != nil || id != c.ID {
		t.Fatalf("human last active = %q %v, want %q", id, err, c.ID)
	}
	if _, _, err := e.resolve(unpinned, u, ""); !errors.Is(err, ErrAmbiguousWorkspace) {
		t.Fatalf("agent with several workspaces and no pin = %v, want ErrAmbiguousWorkspace (it must not inherit last-active)", err)
	}

	// 4. Sole membership: unambiguous for anyone, even with no last-active.
	solo := e.user()
	s := e.workspace(solo.ID)
	if id, _, err := e.resolve(e.sessionCtx(solo.ID), solo, ""); err != nil || id != s.ID {
		t.Fatalf("human sole membership = %q %v", id, err)
	}
	if id, _, err := e.resolve(e.bearerCtx(e.token(solo.ID, "")), solo, ""); err != nil || id != s.ID {
		t.Fatalf("agent sole membership = %q %v", id, err)
	}

	// 5. A human with several workspaces and no last-active still gets one.
	multi := e.user()
	m1 := e.workspace(multi.ID)
	e.workspace(multi.ID)
	if id, _, err := e.resolve(e.sessionCtx(multi.ID), multi, ""); err != nil || id == "" {
		t.Fatalf("human default = %q %v", id, err)
	} else if id != m1.ID {
		t.Logf("human default is %q (first membership is %q)", id, m1.ID)
	}

	// 6. No membership at all.
	none := e.user()
	if _, _, err := e.resolve(e.sessionCtx(none.ID), none, ""); !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("no membership = %v, want ErrNoWorkspace", err)
	}
}

func TestResolveWorkspaceRefusesStrangersAndPinMismatch(t *testing.T) {
	e := newEnv(t)
	u := e.user()
	other := e.user()
	a := e.workspace(u.ID)
	b := e.workspace(u.ID)
	foreign := e.workspace(other.ID)
	pinnedA := e.bearerCtx(e.token(u.ID, a.ID))
	human := e.sessionCtx(u.ID)

	// A pinned token cannot be talked into another workspace, by any spelling,
	// even one its owner belongs to.
	for _, ref := range []string{b.ID, b.Slug, b.KeyPrefix} {
		if _, _, err := e.resolve(pinnedA, u, ref); !errors.Is(err, store.ErrNotMember) {
			t.Fatalf("pinned to A naming %q = %v, want ErrNotMember", ref, err)
		}
	}
	// A caller who names a workspace they are not in is refused, never handed
	// a different one.
	if _, _, err := e.resolve(human, u, foreign.ID); !errors.Is(err, store.ErrNotMember) {
		t.Fatalf("human naming a stranger's workspace = %v, want ErrNotMember", err)
	}
	if _, _, err := e.resolve(human, u, "no-such-workspace"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown workspace = %v, want ErrNotFound", err)
	}
	// A pin to a workspace the owner has left is refused too.
	if _, err := e.pool.Exec(context.Background(),
		`DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.resolve(pinnedA, u, ""); !errors.Is(err, store.ErrNotMember) {
		t.Fatalf("pin after leaving = %v, want ErrNotMember", err)
	}
	if !IsAccessError(store.ErrNotMember) || !IsAccessError(ErrAmbiguousWorkspace) || IsAccessError(errors.New("db down")) {
		t.Fatal("IsAccessError misclassifies")
	}
}

func TestCheckCSRF(t *testing.T) {
	m := NewManager(nil, false)
	req := func(cookie, header string) *http.Request {
		r := httptest.NewRequest("POST", "/x", nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: CSRFCookie, Value: cookie})
		}
		if header != "" {
			r.Header.Set(CSRFHeader, header)
		}
		return r
	}
	if !m.CheckCSRF(req("tok", "tok")) {
		t.Fatal("matching pair refused")
	}
	for name, r := range map[string]*http.Request{
		"no cookie":  req("", "tok"),
		"no header":  req("tok", ""),
		"mismatch":   req("tok", "other"),
		"both empty": req("", ""),
	} {
		if m.CheckCSRF(r) {
			t.Fatalf("%s accepted", name)
		}
	}
}
