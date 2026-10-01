package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"raenil/internal/auth"
	"raenil/internal/store"
)

// ---- simple in-memory login rate limiter (per client IP) ----

// sweepInterval is how often idle keys are dropped, so a flood of distinct
// client addresses cannot grow the map forever.
const sweepInterval = time.Minute

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
	now    func() time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, max: max, window: window, now: time.Now}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	cutoff := now.Add(-rl.window)
	kept := rl.hits[key][:0]
	for _, t := range rl.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.hits[key] = kept
		return false
	}
	rl.hits[key] = append(kept, now)
	return true
}

// sweep forgets every key with no hit inside the window.
func (rl *rateLimiter) sweep() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := rl.now().Add(-rl.window)
	for key, ts := range rl.hits {
		live := false
		for _, t := range ts {
			if t.After(cutoff) {
				live = true
				break
			}
		}
		if !live {
			delete(rl.hits, key)
		}
	}
}

func (rl *rateLimiter) size() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.hits)
}

// run sweeps every interval until ctx is cancelled.
func (rl *rateLimiter) run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rl.sweep()
		}
	}
}

// clientIP names the caller for rate limiting. Headers are attacker-controlled
// unless a proxy we trust sets them, so by default only the TCP peer counts.
// With RAENIL_TRUSTED_PROXY_HEADER set, that one header is believed instead:
// X-Forwarded-For contributes its right-most entry (the one our proxy appended;
// everything left of it came from the client), any other header (for example
// CF-Connecting-IP) is taken as the address itself. A missing or malformed
// value falls back to the peer, which fails closed: callers share a bucket.
func clientIP(r *http.Request, trustedHeader string) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if trustedHeader == "" {
		return peer
	}
	v := strings.TrimSpace(r.Header.Get(trustedHeader))
	if strings.EqualFold(trustedHeader, "X-Forwarded-For") {
		// Several header lines are joined, so take the last entry overall.
		v = strings.Join(r.Header.Values(trustedHeader), ",")
		parts := strings.Split(v, ",")
		v = strings.TrimSpace(parts[len(parts)-1])
	}
	if ip := net.ParseIP(v); ip != nil {
		return ip.String()
	}
	return peer
}

// A login for an unknown email still has to pay for one argon2 verification,
// or the response time tells an attacker which emails have accounts.
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

const dummyPassword = "raenil-dummy-password-never-matches"

func loginDummyHash() string {
	dummyHashOnce.Do(func() {
		h, err := auth.HashPassword(dummyPassword)
		if err != nil {
			// Unreachable short of the OS RNG failing; verifying against an
			// empty hash would silently drop the timing protection.
			panic("api: cannot build dummy password hash: " + err.Error())
		}
		dummyHash = h
	})
	return dummyHash
}

// ---- handlers ----

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.UserCount(r.Context())
	if handleStoreErr(w, err) {
		return
	}
	resp := map[string]any{"setupRequired": n == 0, "authenticated": false}
	if u, ok := auth.UserFrom(r.Context()); ok {
		resp["authenticated"] = true
		resp["user"] = u
	}
	writeJSON(w, 200, resp)
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.UserCount(r.Context())
	if handleStoreErr(w, err) {
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, "already_set_up")
		return
	}
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	c.Email = strings.TrimSpace(c.Email)
	if c.Email == "" || len(c.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "email required, password >= 8 chars")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		internalErr(w, err)
		return
	}
	// The count above only saves a hash on the common path. The insert itself
	// is what guarantees a single first user, even under concurrent requests.
	u, err := s.store.CreateFirstUser(r.Context(), c.Email, hash)
	if errors.Is(err, store.ErrConflict) {
		writeErr(w, http.StatusConflict, "already_set_up")
		return
	}
	if handleStoreErr(w, err) {
		return
	}
	if err := s.grantSession(w, r, u.ID); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.allow(clientIP(r, s.cfg.TrustedProxyHeader)) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, slow down")
		return
	}
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, hash, err := s.store.GetUserByEmail(r.Context(), strings.TrimSpace(c.Email))
	if errors.Is(err, store.ErrNotFound) {
		// Same work as a wrong password, same answer.
		s.verifyPassword(loginDummyHash(), c.Password)
		writeErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if handleStoreErr(w, err) {
		return
	}
	if !s.verifyPassword(hash, c.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err := s.grantSession(w, r, u.ID); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, u)
}

// grantSession sets both the session and CSRF cookies. It reports failure so a
// caller never answers success without a cookie to back it.
func (s *Server) grantSession(w http.ResponseWriter, r *http.Request, userID string) error {
	cookie, err := s.auth.StartSession(r.Context(), userID)
	if err != nil {
		return err
	}
	csrf, err := s.auth.NewCSRFCookie()
	if err != nil {
		return err
	}
	http.SetCookie(w, cookie)
	http.SetCookie(w, csrf)
	return nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = s.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, s.auth.ClearCookie())
	writeJSON(w, 200, map[string]string{"status": "logged out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	out := map[string]any{"id": u.ID, "email": u.Email, "createdAt": u.CreatedAt}
	// Report where the caller currently is, so the client can render the
	// switcher without a second round trip. Absent when they belong to none.
	if wsp, role, err := s.auth.ResolveWorkspace(r.Context(), u, auth.RequestedWorkspace(r)); err == nil {
		out["workspace"] = wsp
		out["role"] = role
	}
	writeJSON(w, 200, out)
}

// ---- API tokens ----

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	tokens, err := s.store.ListAPITokens(r.Context(), u.ID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, tokens)
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		Name string `json:"name"`
		// Workspace pins the token to one workspace (id, slug or key prefix),
		// so an agent working in one repo cannot read another's issues.
		Workspace string `json:"workspace"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var pin *string
	if strings.TrimSpace(body.Workspace) != "" {
		target, err := s.store.ResolveWorkspace(r.Context(), body.Workspace)
		if handleStoreErr(w, err) {
			return
		}
		// You may only pin a token to a workspace you belong to.
		if _, err := s.store.RoleIn(r.Context(), target.ID, u.ID); handleStoreErr(w, err) {
			return
		}
		pin = &target.ID
	}
	if strings.TrimSpace(body.Name) == "" {
		body.Name = "token"
	}
	raw, err := auth.RandomToken(32)
	if err != nil {
		internalErr(w, err)
		return
	}
	plaintext := "raenil_" + raw
	t, err := s.store.CreateAPIToken(r.Context(), u.ID, body.Name, auth.HashToken(plaintext), pin)
	if handleStoreErr(w, err) {
		return
	}
	// The plaintext token is returned exactly once.
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": plaintext})
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if handleStoreErr(w, s.store.DeleteAPIToken(r.Context(), u.ID, r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
