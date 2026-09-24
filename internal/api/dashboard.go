package api

import (
	"net/http"
	"time"
)

// handleDashboard serves the workspace overview. ?tz names the viewer's
// timezone (IANA, e.g. Asia/Yangon) so its days are the viewer's days.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tz := time.UTC
	if name := r.URL.Query().Get("tz"); name != "" {
		loc, err := time.LoadLocation(name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "unknown timezone "+name)
			return
		}
		tz = loc
	}
	d, err := s.store.Dashboard(r.Context(), ws(r), tz)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, d)
}
