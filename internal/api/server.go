// Package api exposes the REST + SSE HTTP surface and serves the static PWA.
package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"raenil/internal/auth"
	"raenil/internal/config"
	"raenil/internal/events"
	"raenil/internal/service"
	"raenil/internal/sse"
	"raenil/internal/store"
)

type Server struct {
	cfg       config.Config
	store     *store.Store
	svc       *service.Service
	bus       *events.Bus
	auth      *auth.Manager
	sse       *sse.Handler
	mcp       http.Handler // mounted at /mcp (may be nil)
	staticDir string
}

func NewServer(cfg config.Config, st *store.Store, svc *service.Service, bus *events.Bus, mcp http.Handler) *Server {
	return &Server{
		cfg:       cfg,
		store:     st,
		svc:       svc,
		bus:       bus,
		auth:      auth.NewManager(st, cfg.SecureCookies()),
		sse:       sse.NewHandler(bus),
		mcp:       mcp,
		staticDir: "web/build",
	}
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// guard requires an authenticated user and enforces CSRF on cookie-authed
// mutations. Bearer (API/MCP) callers skip CSRF since they carry no ambient auth.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFrom(r.Context()); !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !safeMethod(r.Method) && auth.ActorFrom(r.Context()) == auth.ActorHuman {
			if !s.auth.CheckCSRF(r) {
				writeErr(w, http.StatusForbidden, "invalid csrf token")
				return
			}
		}
		next(w, r)
	}
}

// Handler builds the full HTTP handler (routes + auth middleware + static).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health + public config.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/config", s.handleConfig)

	// Auth (public).
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.guard(s.handleMe))

	// API tokens.
	mux.HandleFunc("GET /api/tokens", s.guard(s.handleListTokens))
	mux.HandleFunc("POST /api/tokens", s.guard(s.handleCreateToken))
	mux.HandleFunc("DELETE /api/tokens/{id}", s.guard(s.handleDeleteToken))

	// Metadata.
	mux.HandleFunc("GET /api/states", s.guard(s.handleListStates))
	mux.HandleFunc("GET /api/states/{id}", s.guard(s.handleGetState))
	mux.HandleFunc("GET /api/labels", s.guard(s.handleListLabels))
	mux.HandleFunc("GET /api/label-groups", s.guard(s.handleListLabelGroups))
	mux.HandleFunc("POST /api/labels", s.guard(s.handleCreateLabel))

	// Initiatives.
	mux.HandleFunc("GET /api/initiatives", s.guard(s.handleListInitiatives))
	mux.HandleFunc("POST /api/initiatives", s.guard(s.handleSaveInitiative))
	mux.HandleFunc("GET /api/initiatives/{id}", s.guard(s.handleGetInitiative))
	mux.HandleFunc("PATCH /api/initiatives/{id}", s.guard(s.handleSaveInitiative))
	mux.HandleFunc("DELETE /api/initiatives/{id}", s.guard(s.handleDeleteInitiative))

	// Projects.
	mux.HandleFunc("GET /api/projects", s.guard(s.handleListProjects))
	mux.HandleFunc("POST /api/projects", s.guard(s.handleSaveProject))
	mux.HandleFunc("GET /api/projects/{id}", s.guard(s.handleGetProject))
	mux.HandleFunc("PATCH /api/projects/{id}", s.guard(s.handleSaveProject))
	mux.HandleFunc("DELETE /api/projects/{id}", s.guard(s.handleDeleteProject))

	// Issues.
	mux.HandleFunc("GET /api/issues", s.guard(s.handleListIssues))
	mux.HandleFunc("POST /api/issues", s.guard(s.handleCreateIssue))
	mux.HandleFunc("GET /api/issues/{id}", s.guard(s.handleGetIssue))
	mux.HandleFunc("PATCH /api/issues/{id}", s.guard(s.handleUpdateIssue))
	mux.HandleFunc("DELETE /api/issues/{id}", s.guard(s.handleDeleteIssue))
	mux.HandleFunc("GET /api/issues/{id}/comments", s.guard(s.handleListComments))
	mux.HandleFunc("POST /api/issues/{id}/comments", s.guard(s.handleAddComment))

	// Documents.
	mux.HandleFunc("GET /api/documents", s.guard(s.handleListDocuments))
	mux.HandleFunc("POST /api/documents", s.guard(s.handleSaveDocument))
	mux.HandleFunc("GET /api/documents/{id}", s.guard(s.handleGetDocument))
	mux.HandleFunc("PATCH /api/documents/{id}", s.guard(s.handleSaveDocument))
	mux.HandleFunc("DELETE /api/documents/{id}", s.guard(s.handleDeleteDocument))

	// Push.
	// bulk import (external tracker → Raenil)
	mux.HandleFunc("POST /api/import", s.guard(s.handleImport))

	mux.HandleFunc("POST /api/push/subscribe", s.guard(s.handlePushSubscribe))
	mux.HandleFunc("POST /api/push/unsubscribe", s.guard(s.handlePushUnsubscribe))

	// SSE.
	mux.HandleFunc("GET /api/events", s.guard(s.sse.ServeHTTP))

	// MCP endpoint (bearer-authed inside the handler).
	if s.mcp != nil {
		mux.Handle("/mcp", s.mcp)
		mux.Handle("/mcp/", s.mcp)
	}

	// Static PWA + SPA fallback for everything else.
	mux.HandleFunc("/", s.serveStatic)

	return s.auth.Middleware(mux)
}

// serveStatic serves files from the build dir, falling back to index.html so
// client-side routes resolve (SPA). API/MCP paths never reach here.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/mcp") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	clean := filepath.Clean(r.URL.Path)
	full := filepath.Join(s.staticDir, clean)
	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		if strings.HasSuffix(clean, ".webmanifest") {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		// The service worker must be served from the root scope uncached.
		if clean == "/sw.js" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, full)
		return
	}
	index := filepath.Join(s.staticDir, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	// Frontend not built yet.
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "raenil",
		"note":    "frontend not built; run the web build. API is under /api",
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"vapidPublicKey": s.cfg.VAPIDPublic,
		"pushEnabled":    s.cfg.PushEnabled(),
		"issuePrefix":    s.cfg.IssuePrefix,
		"baseUrl":        s.cfg.BaseURL,
	})
}
