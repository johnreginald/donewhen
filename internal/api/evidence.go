package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"raenil/internal/events"
)

// evidenceTypes are the files a ticket's evidence may hold.
var evidenceTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif",
	".url": "text/uri-list", ".txt": "text/plain", ".md": "text/markdown",
}

// handlePutEvidence replaces a ticket's evidence with the files the runner
// host uploads: {"files": {"name.png": "<base64>", ...}}.
func (s *Server) handlePutEvidence(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	var body struct {
		Files map[string]string `json:"files"`
	}
	// Screenshots are larger than any other body: 64 MiB for the set.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	files, types := map[string][]byte{}, map[string]string{}
	for name, b64 := range body.Files {
		name = path.Base(name)
		ct, ok := evidenceTypes[strings.ToLower(path.Ext(name))]
		if !ok {
			writeErr(w, http.StatusBadRequest, "evidence cannot hold "+name)
			return
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, name+": "+err.Error())
			return
		}
		files[name], types[name] = data, ct
	}
	n, err := s.store.ReplaceEvidence(r.Context(), ws(r), id, files, types)
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "evidence.saved", IssueID: id})
	writeJSON(w, http.StatusOK, map[string]int{"files": n})
}

func (s *Server) handleListEvidence(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	out, err := s.store.ListEvidence(r.Context(), ws(r), id)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(out))
}

// handleEvidenceFile serves one file, e.g. a screenshot for an <img>.
func (s *Server) handleEvidenceFile(w http.ResponseWriter, r *http.Request) {
	ct, data, err := s.store.EvidenceFile(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}
