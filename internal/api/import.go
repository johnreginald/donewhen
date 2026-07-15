package api

import (
	"encoding/json"
	"net/http"

	"raenil/internal/store"
)

// handleImport bulk-loads an external tracker export (Linear-native JSON shape)
// into Raenil, preserving keys/timestamps/state/labels. Auth: any authenticated
// caller (session or bearer). The body is decoded leniently (Linear objects
// carry many fields we ignore) with a generous size cap.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)) // 16 MiB
	var data store.ImportData
	if err := dec.Decode(&data); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid import payload: "+err.Error())
		return
	}
	res, err := s.store.Import(r.Context(), data)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}
