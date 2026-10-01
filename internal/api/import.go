package api

import (
	"encoding/json"
	"net/http"

	"github.com/johnreginald/donewhen/internal/store"
)

// handleImport bulk-loads an external tracker export (Linear-native JSON shape)
// into DoneWhen, preserving keys/timestamps/state/labels. Auth: owner/admin
// browser session only (see adminSessionOnly). The body is decoded leniently (Linear objects
// carry many fields we ignore) with a generous size cap.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)) // 16 MiB
	var data store.ImportData
	if err := dec.Decode(&data); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid import payload: "+err.Error())
		return
	}
	res, err := s.store.Import(r.Context(), ws(r), data)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleUpdateDescriptions backfills full issue descriptions by key
// ({"PP-80":"full markdown", ...}). Used after an import whose source
// (Linear list_issues) truncated long bodies.
func (s *Server) handleUpdateDescriptions(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
	var byKey map[string]string
	if err := dec.Decode(&byKey); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}
	n, err := s.store.UpdateDescriptions(r.Context(), ws(r), byKey)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}
