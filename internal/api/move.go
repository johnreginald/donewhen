package api

import (
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/store"
)

type issueMoveReq struct {
	State  string  `json:"state"`  // workflow state name or id; empty = stay in the column
	After  *string `json:"after"`  // card above the drop point
	Before *string `json:"before"` // card below the drop point
	Force  bool    `json:"force"`
}

// handleMoveIssue moves one card in a single request: it changes the column
// and sets the position between the two neighbours the client saw, in one
// transaction. It goes through the same service path as PATCH, so the
// done-when gate, the activity row and the bus event are the same.
func (s *Server) handleMoveIssue(w http.ResponseWriter, r *http.Request) {
	var req issueMoveReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	cur, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	p := store.IssuePatch{
		Rank:      &store.Rank{After: derefStr(req.After), Before: derefStr(req.Before)},
		ForceGate: req.Force && canForceGate(r),
	}
	if req.State != "" {
		if looksLikeKey(req.State) {
			p.StateName = &req.State
		} else {
			p.StateID = &req.State
		}
	}
	is, err := s.svc.UpdateIssue(r.Context(), ws(r), cur.ID, p, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, is)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
