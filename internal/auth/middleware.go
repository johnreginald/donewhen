package auth

import (
	"context"
	"crypto/subtle"
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
	SessionTTL    = 30 * 24 * time.Hour
	ActorHuman    = "human"
	ActorAI       = "ai"
)

type ctxKey int

const (
	userKey ctxKey = iota
	actorKey
)

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
				if u, err := m.Store.LookupAPIToken(ctx, HashToken(tok)); err == nil {
					ctx = context.WithValue(ctx, userKey, u)
					ctx = context.WithValue(ctx, actorKey, ActorAI)
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
