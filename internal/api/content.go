package api

import (
	"net/http"

	"raenil/internal/auth"
	"raenil/internal/models"
)

// ---- comments ----

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	comments, err := s.store.ListComments(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(comments))
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BodyMd string `json:"bodyMd"`
	}
	if err := readJSON(r, &body); err != nil || body.BodyMd == "" {
		writeErr(w, http.StatusBadRequest, "bodyMd required")
		return
	}
	c, err := s.svc.AddComment(r.Context(), r.PathValue("id"), body.BodyMd, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ---- documents ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.ListDocuments(r.Context(), r.URL.Query().Get("project"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(docs))
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDocument(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) handleSaveDocument(w http.ResponseWriter, r *http.Request) {
	var d models.Document
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if id := r.PathValue("id"); id != "" {
		d.ID = id
	}
	if d.Title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	saved, err := s.store.SaveDocument(r.Context(), d)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteDocument(r.Context(), r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
