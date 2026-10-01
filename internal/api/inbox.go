package api

import (
	"net/http"
	"time"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/store"
)

// handleInbox returns the review queue + recent AI activity for the landing
// "Inbox" — the human's read-back of what the AI did.
func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())

	needs, err := s.store.InboxNeedsReview(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	recent, err := s.store.ListRecentActivity(r.Context(), store.ActivityFilter{WorkspaceID: ws(r), Actor: "ai", Limit: 100})
	if handleStoreErr(w, err) {
		return
	}
	var seenAt *time.Time
	if u.ID != "" {
		seenAt, _ = s.store.GetInboxSeen(r.Context(), u.ID)
	}
	writeJSON(w, 200, map[string]any{
		"needsReview": orEmpty(needs),
		"recent":      orEmpty(recent),
		"seenAt":      seenAt,
	})
}

// handleInboxSeen stamps the inbox as reviewed now (clears the badge).
func (s *Server) handleInboxSeen(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	t, err := s.store.SetInboxSeen(r.Context(), u.ID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"seenAt": t})
}
