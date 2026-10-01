// Package api exposes the REST + SSE HTTP surface and serves the static PWA.
package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/push"
	"github.com/johnreginald/donewhen/internal/service"
	"github.com/johnreginald/donewhen/internal/sse"
	"github.com/johnreginald/donewhen/internal/store"
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

	loginLimiter *rateLimiter
	// verifyPassword is auth.VerifyPassword; a field so tests can observe it.
	verifyPassword func(hash, password string) bool
	stop           context.CancelFunc
	// pushResolve resolves push endpoint hosts; nil means the system resolver.
	pushResolve push.Resolver
}

func NewServer(cfg config.Config, st *store.Store, svc *service.Service, bus *events.Bus, mcp http.Handler) *Server {
	sweepCtx, stop := context.WithCancel(context.Background())
	loginDummyHash() // pay the one-off argon2 cost now, not on the first miss
	s := &Server{
		cfg:       cfg,
		store:     st,
		svc:       svc,
		bus:       bus,
		auth:      auth.NewManager(st, cfg.SecureCookies()),
		mcp:       mcp,
		staticDir: "web/build",

		loginLimiter:   newRateLimiter(10, time.Minute),
		verifyPassword: auth.VerifyPassword,
		stop:           stop,
	}
	s.sse = sse.NewHandler(bus, s.auth.Revalidate)
	go s.loginLimiter.run(sweepCtx, sweepInterval)
	return s
}

// Close stops the server's background sweeper.
func (s *Server) Close() { s.stop() }

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

// sessionOnly is guard for account-level actions that a bearer token must never
// perform, pinned or not: it refuses API/MCP callers outright.
func (s *Server) sessionOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request) {
		if auth.IsBearer(r.Context()) {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	})
}

// wsGuard is guard plus tenancy: it resolves which workspace the request acts
// on and proves membership before the handler runs. Everything that reads or
// writes tenant data goes through here.
func (s *Server) wsGuard(next http.HandlerFunc) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFrom(r.Context())
		wsp, role, err := s.auth.ResolveWorkspace(r.Context(), user, auth.RequestedWorkspace(r))
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrNoWorkspace):
				writeErr(w, http.StatusForbidden, "no workspace available for this account")
			case errors.Is(err, auth.ErrAmbiguousWorkspace):
				writeErr(w, http.StatusBadRequest,
					"this token spans several workspaces; name one with the X-Workspace header")
			case errors.Is(err, store.ErrNotFound):
				writeErr(w, http.StatusNotFound, "workspace not found")
			default:
				// Membership failures and pinned-token mismatches both land
				// here: the caller may not act on the workspace it named.
				writeErr(w, http.StatusForbidden, "not a member of this workspace")
			}
			return
		}
		next(w, r.WithContext(auth.WithWorkspace(r.Context(), wsp, role)))
	})
}

// adminOnly rejects a member trying to administer a workspace. The UI hides
// these actions, but hiding a button is not access control.
func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return s.wsGuard(func(w http.ResponseWriter, r *http.Request) {
		if !canAdmin(r) {
			writeErr(w, http.StatusForbidden, "requires workspace owner or admin")
			return
		}
		next(w, r)
	})
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

	// Workspaces — the tenancy boundary. Listing memberships must NOT be
	// workspace-scoped: it is how a client discovers which ones exist.
	mux.HandleFunc("GET /api/workspaces", s.guard(s.handleListWorkspaces))
	mux.HandleFunc("POST /api/workspaces", s.sessionOnly(s.handleCreateWorkspace))
	mux.HandleFunc("POST /api/workspaces/{id}/activate", s.sessionOnly(s.handleActivateWorkspace))
	mux.HandleFunc("PATCH /api/workspaces/{id}", s.adminOnly(s.handleUpdateWorkspace))
	mux.HandleFunc("GET /api/workspaces/{id}/members", s.wsGuard(s.handleListMembers))
	mux.HandleFunc("POST /api/workspaces/{id}/members", s.adminOnly(s.handleAddMember))
	mux.HandleFunc("DELETE /api/workspaces/{id}/members/{userId}", s.adminOnly(s.handleRemoveMember))

	// API tokens.
	mux.HandleFunc("GET /api/tokens", s.sessionOnly(s.handleListTokens))
	mux.HandleFunc("POST /api/tokens", s.sessionOnly(s.handleCreateToken))
	mux.HandleFunc("DELETE /api/tokens/{id}", s.sessionOnly(s.handleDeleteToken))

	// Metadata.
	mux.HandleFunc("GET /api/states", s.wsGuard(s.handleListStates))
	mux.HandleFunc("GET /api/states/{id}", s.wsGuard(s.handleGetState))
	mux.HandleFunc("GET /api/labels", s.wsGuard(s.handleListLabels))
	mux.HandleFunc("GET /api/label-groups", s.wsGuard(s.handleListLabelGroups))
	mux.HandleFunc("POST /api/labels", s.wsGuard(s.handleCreateLabel))

	// Initiatives.
	mux.HandleFunc("GET /api/initiatives", s.wsGuard(s.handleListInitiatives))
	mux.HandleFunc("POST /api/initiatives", s.wsGuard(s.handleSaveInitiative))
	mux.HandleFunc("GET /api/initiatives/{id}", s.wsGuard(s.handleGetInitiative))
	mux.HandleFunc("PATCH /api/initiatives/{id}", s.wsGuard(s.handleSaveInitiative))
	mux.HandleFunc("DELETE /api/initiatives/{id}", s.wsGuard(s.handleDeleteInitiative))

	// Projects.
	mux.HandleFunc("GET /api/projects", s.wsGuard(s.handleListProjects))
	mux.HandleFunc("POST /api/projects", s.wsGuard(s.handleSaveProject))
	mux.HandleFunc("GET /api/projects/{id}", s.wsGuard(s.handleGetProject))
	mux.HandleFunc("PATCH /api/projects/{id}", s.wsGuard(s.handleSaveProject))
	mux.HandleFunc("POST /api/projects/{id}/archive", s.wsGuard(s.handleArchiveProject))
	mux.HandleFunc("POST /api/projects/{id}/unarchive", s.wsGuard(s.handleUnarchiveProject))
	mux.HandleFunc("DELETE /api/projects/{id}", s.wsGuard(s.handleDeleteProject))

	// Issues.
	mux.HandleFunc("GET /api/issues/missing-docs", s.wsGuard(s.handleMissingDocs))
	mux.HandleFunc("GET /api/issues", s.wsGuard(s.handleListIssues))
	mux.HandleFunc("POST /api/issues", s.wsGuard(s.handleCreateIssue))
	mux.HandleFunc("GET /api/issues/{id}", s.wsGuard(s.handleGetIssue))
	mux.HandleFunc("PATCH /api/issues/{id}", s.wsGuard(s.handleUpdateIssue))
	mux.HandleFunc("DELETE /api/issues/{id}", s.wsGuard(s.handleDeleteIssue))
	mux.HandleFunc("GET /api/issues/{id}/activity", s.wsGuard(s.handleIssueActivity))
	mux.HandleFunc("GET /api/activity", s.wsGuard(s.handleActivity))

	// inbox — the human's review queue (AI moved to In Review) + recent AI activity
	mux.HandleFunc("GET /api/inbox", s.wsGuard(s.handleInbox))
	mux.HandleFunc("POST /api/inbox/seen", s.wsGuard(s.handleInboxSeen))

	// dev links (branch / PR / commits) + done-when criteria
	mux.HandleFunc("GET /api/issues/{id}/commits", s.wsGuard(s.handleListCommits))
	mux.HandleFunc("POST /api/issues/{id}/commits", s.wsGuard(s.handleAddCommit))
	// Reverse lookup: which issue owns this commit, and what was it meant
	// to satisfy. The seam for code-intelligence tooling — see dev.go.
	mux.HandleFunc("GET /api/commits/{sha}", s.wsGuard(s.handleIssueByCommit))
	mux.HandleFunc("PATCH /api/issues/{id}/dev", s.wsGuard(s.handleSetDev))
	mux.HandleFunc("GET /api/issues/{id}/criteria", s.wsGuard(s.handleListCriteria))
	mux.HandleFunc("POST /api/issues/{id}/criteria", s.wsGuard(s.handleAddCriterion))
	mux.HandleFunc("PATCH /api/criteria/{id}", s.wsGuard(s.handleUpdateCriterion))
	mux.HandleFunc("DELETE /api/criteria/{id}", s.wsGuard(s.handleDeleteCriterion))
	mux.HandleFunc("GET /api/issues/{id}/blockers", s.wsGuard(s.handleGetBlockers))
	mux.HandleFunc("PUT /api/issues/{id}/blockers", s.wsGuard(s.handleSetBlockers))
	mux.HandleFunc("GET /api/blockers", s.wsGuard(s.handleListBlockLinks))
	mux.HandleFunc("GET /api/issues/{id}/comments", s.wsGuard(s.handleListComments))
	mux.HandleFunc("POST /api/issues/{id}/comments", s.wsGuard(s.handleAddComment))

	// Documents.
	mux.HandleFunc("GET /api/documents", s.wsGuard(s.handleListDocuments))
	mux.HandleFunc("POST /api/documents", s.wsGuard(s.handleSaveDocument))
	mux.HandleFunc("GET /api/documents/{id}", s.wsGuard(s.handleGetDocument))
	mux.HandleFunc("PATCH /api/documents/{id}", s.wsGuard(s.handleSaveDocument))
	mux.HandleFunc("DELETE /api/documents/{id}", s.wsGuard(s.handleDeleteDocument))

	// Push.
	// bulk import (external tracker → DoneWhen)
	mux.HandleFunc("POST /api/import", s.wsGuard(s.handleImport))
	mux.HandleFunc("POST /api/import/descriptions", s.wsGuard(s.handleUpdateDescriptions))

	mux.HandleFunc("POST /api/push/subscribe", s.guard(s.handlePushSubscribe))
	mux.HandleFunc("POST /api/push/unsubscribe", s.guard(s.handlePushUnsubscribe))

	// SSE.
	mux.HandleFunc("GET /api/events", s.wsGuard(s.sse.ServeHTTP))

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
		"service": "donewhen",
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
