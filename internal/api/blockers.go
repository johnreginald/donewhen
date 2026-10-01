package api

import (
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/events"
)

// ---- "blocked by" links ----

func (s *Server) handleGetBlockers(w http.ResponseWriter, r *http.Request) {
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	s.writeBlockers(w, r, is.ID)
}

func (s *Server) writeBlockers(w http.ResponseWriter, r *http.Request, issueID string) {
	by, err := s.store.ListBlockers(r.Context(), ws(r), issueID)
	if handleStoreErr(w, err) {
		return
	}
	blocking, err := s.store.ListBlocking(r.Context(), ws(r), issueID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blockedBy": by, "blocking": blocking})
}

// handleSetBlockers replaces the tickets that block one ticket.
func (s *Server) handleSetBlockers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BlockedBy []string `json:"blockedBy"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	if _, err := s.store.SetBlockers(r.Context(), ws(r), is.ID, body.BlockedBy); handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "issue.blockers", IssueID: is.ID})
	s.writeBlockers(w, r, is.ID)
}

// handleListBlockLinks lists every "blocked by" edge in the workspace, for
// the board.
func (s *Server) handleListBlockLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.store.ListBlockLinks(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// publish sends an event about the request's workspace to live pages.
func (s *Server) publish(r *http.Request, e events.Event) {
	e.WorkspaceID = ws(r)
	if e.Actor == "" {
		e.Actor = auth.ActorFrom(r.Context())
	}
	s.bus.Publish(e)
}
