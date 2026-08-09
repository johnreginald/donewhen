package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"raenil/internal/auth"
)

// ---- simple in-memory login rate limiter (per client IP) ----

type rateLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	max     int
	window  time.Duration
}

var loginLimiter = &rateLimiter{hits: map[string][]time.Time{}, max: 10, window: time.Minute}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
		writeErr(w, http.StatusConflict, "already set up")
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
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := s.store.CreateUser(r.Context(), c.Email, hash)
	if handleStoreErr(w, err) {
		return
	}
	s.grantSession(w, r, u.ID)
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !loginLimiter.allow(clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, slow down")
		return
	}
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, hash, err := s.store.GetUserByEmail(r.Context(), strings.TrimSpace(c.Email))
	if err != nil || !auth.VerifyPassword(hash, c.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.grantSession(w, r, u.ID)
	writeJSON(w, 200, u)
}

// grantSession sets both the session and CSRF cookies.
func (s *Server) grantSession(w http.ResponseWriter, r *http.Request, userID string) {
	if cookie, err := s.auth.StartSession(r.Context(), userID); err == nil {
		http.SetCookie(w, cookie)
	}
	if csrf, err := s.auth.NewCSRFCookie(); err == nil {
		http.SetCookie(w, csrf)
	}
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
		writeErr(w, http.StatusInternalServerError, err.Error())
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
