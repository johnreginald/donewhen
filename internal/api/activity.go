package api

import (
	"net/http"
	"strconv"
	"strings"

	"raenil/internal/store"
)

// handleIssueActivity returns one issue's timeline (accepts id or key).
func (s *Server) handleIssueActivity(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	is, err := s.store.GetIssue(r.Context(), ws(r), ref)
	if err != nil && strings.Contains(ref, "-") {
		is, err = s.store.GetIssueByKey(r.Context(), ws(r), ref)
	}
	if handleStoreErr(w, err) {
		return
	}
	acts, err := s.store.ListActivity(r.Context(), ws(r), is.ID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(acts))
}

// handleMissingDocs returns completed issues with no attached artifact — the
// gaps in the record.
func (s *Server) handleMissingDocs(w http.ResponseWriter, r *http.Request) {
	iss, err := s.store.IssuesMissingDocs(r.Context(), []string{ws(r)})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(iss))
}

// handleActivity returns the cross-issue log feed, filterable by project /
// initiative / actor.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	acts, err := s.store.ListRecentActivity(r.Context(), store.ActivityFilter{
		WorkspaceID:  ws(r),
		InitiativeID: q.Get("initiative"),
		ProjectID:    q.Get("project"),
		Actor:        q.Get("actor"),
		Limit:        limit,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(acts))
}
