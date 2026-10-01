package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"raenil/internal/models"
	"raenil/internal/store"
)

const (
	SessionCookie = "raenil_session"
	CSRFCookie    = "raenil_csrf"
	CSRFHeader    = "X-CSRF-Token"
	// WorkspaceHeader names the workspace a request acts on, by id, slug or key
	// prefix. The ?workspace= query parameter is accepted as an alternative for
	// EventSource, which cannot set headers.
	WorkspaceHeader = "X-Workspace"
	WorkspaceQuery  = "workspace"
	SessionTTL      = 30 * 24 * time.Hour
	ActorHuman      = "human"
	ActorAI         = "ai"
)

type ctxKey int

const (
	userKey ctxKey = iota
	actorKey
	workspaceKey
	roleKey
	tokenPinKey
)

// ErrNoWorkspace means the caller belongs to no workspace at all — nothing to
// scope a request to.
var ErrNoWorkspace = errors.New("no workspace available")

// ErrAmbiguousWorkspace means the caller belongs to several workspaces and did
// not say which one to use. Better to say so than to silently pick.
var ErrAmbiguousWorkspace = errors.New("workspace must be specified")

// Manager wires auth against the store and cookie policy.
type Manager struct {
	Store  *store.Store
	Secure bool
}

func NewManager(s *store.Store, secure bool) *Manager {
	return &Manager{Store: s, Secure: secure}
}

// Middleware resolves the caller (session cookie or bearer token) and stashes
// the user + actor in the request context. It never rejects; use RequireAuth
// to gate protected routes.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Bearer token (API / MCP) => actor "ai".
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			if tok != "" {
				if u, pin, err := m.Store.LookupAPIToken(ctx, HashToken(tok)); err == nil {
					ctx = context.WithValue(ctx, userKey, u)
					ctx = context.WithValue(ctx, actorKey, ActorAI)
					if pin != nil {
						ctx = context.WithValue(ctx, tokenPinKey, *pin)
					}
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Session cookie => actor "human".
		if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
			if u, err := m.Store.LookupSession(ctx, c.Value); err == nil {
				ctx = context.WithValue(ctx, userKey, u)
				ctx = context.WithValue(ctx, actorKey, ActorHuman)
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth returns 401 unless a user is present in context.
func (m *Manager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFrom(r.Context()); !ok {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFrom returns the authenticated user from context, if any.
func UserFrom(ctx context.Context) (models.User, bool) {
	u, ok := ctx.Value(userKey).(models.User)
	return u, ok
}

// ActorFrom returns "human" or "ai" for the current request (default "human").
func ActorFrom(ctx context.Context) string {
	if a, ok := ctx.Value(actorKey).(string); ok && a != "" {
		return a
	}
	return ActorHuman
}

// WorkspaceFrom returns the workspace this request acts on.
func WorkspaceFrom(ctx context.Context) (models.Workspace, bool) {
	w, ok := ctx.Value(workspaceKey).(models.Workspace)
	return w, ok
}

// RoleFrom returns the caller's role in the active workspace.
func RoleFrom(ctx context.Context) string {
	r, _ := ctx.Value(roleKey).(string)
	return r
}

// IsBearer reports whether the caller authenticated with an API token rather
// than a browser session. Account-level actions (minting tokens, creating
// workspaces) are for sessions only, so a leaked agent token cannot widen
// itself.
func IsBearer(ctx context.Context) bool { return ActorFrom(ctx) == ActorAI }

// TokenPinFrom returns the workspace an API token is pinned to, if any.
func TokenPinFrom(ctx context.Context) (string, bool) {
	p, ok := ctx.Value(tokenPinKey).(string)
	return p, ok
}

// WithWorkspace stashes a resolved workspace + role on the context.
func WithWorkspace(ctx context.Context, ws models.Workspace, role string) context.Context {
	ctx = context.WithValue(ctx, workspaceKey, ws)
	return context.WithValue(ctx, roleKey, role)
}

// ResolveWorkspace decides which workspace a request acts on and proves the
// caller belongs to it.
//
// Order: an explicit request (header or query) wins; then a token's pin; then
// where the user was last; then their only membership. A caller who names a
// workspace they are not a member of is refused — never quietly given a
// different one — and a pinned token may not be talked out of its pin.
func (m *Manager) ResolveWorkspace(ctx context.Context, user models.User, requested string) (models.Workspace, string, error) {
	pin, pinned := TokenPinFrom(ctx)

	if requested != "" {
		ws, err := m.Store.ResolveWorkspace(ctx, requested)
		if err != nil {
			return models.Workspace{}, "", err
		}
		if pinned && ws.ID != pin {
			return models.Workspace{}, "", store.ErrNotMember
		}
		role, err := m.Store.RoleIn(ctx, ws.ID, user.ID)
		if err != nil {
			return models.Workspace{}, "", err
		}
		return ws, role, nil
	}

	if pinned {
		ws, err := m.Store.GetWorkspace(ctx, pin)
		if err != nil {
			return models.Workspace{}, "", err
		}
		role, err := m.Store.RoleIn(ctx, ws.ID, user.ID)
		if err != nil {
			return models.Workspace{}, "", err
		}
		return ws, role, nil
	}

	// Where the user was last is a convenience for the browser only. An agent
	// must never inherit it: "wherever the human was last looking" is not the
	// same as "the workspace this tool call means", and silently writing to the
	// wrong tracker is the failure this whole boundary exists to prevent.
	if ActorFrom(ctx) == ActorHuman {
		if last, err := m.Store.LastWorkspace(ctx, user.ID); err == nil && last != "" {
			if ws, err := m.Store.GetWorkspace(ctx, last); err == nil {
				if role, err := m.Store.RoleIn(ctx, ws.ID, user.ID); err == nil {
					return ws, role, nil
				}
			}
		}
	}

	memberships, err := m.Store.ListMemberships(ctx, user.ID)
	if err != nil {
		return models.Workspace{}, "", err
	}
	if len(memberships) == 0 {
		return models.Workspace{}, "", ErrNoWorkspace
	}
	// A sole membership is unambiguous for anyone. Beyond that a human browser
	// gets a sensible default, while an agent is made to say which workspace it
	// means — pin its token, or pass `workspace` on the call.
	if ActorFrom(ctx) == ActorAI && len(memberships) > 1 {
		return models.Workspace{}, "", ErrAmbiguousWorkspace
	}
	return memberships[0].Workspace, memberships[0].Role, nil
}

// RequestedWorkspace pulls the workspace a request names, from the header or
// the query string (EventSource cannot set headers).
func RequestedWorkspace(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get(WorkspaceHeader)); v != "" {
		return v
	}
	return strings.TrimSpace(r.URL.Query().Get(WorkspaceQuery))
}

// IsAccessError reports whether err means "you may not act on that workspace",
// as opposed to an infrastructure failure.
func IsAccessError(err error) bool {
	return errors.Is(err, store.ErrNotMember) || errors.Is(err, store.ErrNotFound) ||
		errors.Is(err, ErrNoWorkspace) || errors.Is(err, ErrAmbiguousWorkspace)
}

// StartSession creates a session row and returns the cookie to set.
func (m *Manager) StartSession(ctx context.Context, userID string) (*http.Cookie, error) {
	tok, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	if err := m.Store.CreateSession(ctx, tok, userID, time.Now().Add(SessionTTL)); err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(SessionTTL),
		MaxAge:   int(SessionTTL.Seconds()),
	}, nil
}

// NewCSRFCookie returns a readable (non-HttpOnly) cookie holding a fresh CSRF
// token for the double-submit pattern.
func (m *Manager) NewCSRFCookie() (*http.Cookie, error) {
	tok, err := RandomToken(24)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     CSRFCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: false,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	}, nil
}

// CheckCSRF validates the double-submit token: the X-CSRF-Token header must
// match the raenil_csrf cookie.
func (m *Manager) CheckCSRF(r *http.Request) bool {
	c, err := r.Cookie(CSRFCookie)
	if err != nil || c.Value == "" {
		return false
	}
	h := r.Header.Get(CSRFHeader)
	return h != "" && subtle.ConstantTimeCompare([]byte(h), []byte(c.Value)) == 1
}

// ClearCookie returns a cookie that expires the session immediately.
func (m *Manager) ClearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}
