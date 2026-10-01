package api

import "net/http"

// handleListBlocked lists every Blocked issue with why it is blocked, oldest
// first: the Blocked page.
func (s *Server) handleListBlocked(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListBlocked(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(items))
}
