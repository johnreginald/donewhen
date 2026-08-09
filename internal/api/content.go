package api

import (
	"net/http"

	"raenil/internal/auth"
	"raenil/internal/models"
	"raenil/internal/store"
)

// ---- comments ----

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	comments, err := s.store.ListComments(r.Context(), ws(r), r.PathValue("id"))
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
	c, err := s.svc.AddComment(r.Context(), ws(r), r.PathValue("id"), body.BodyMd, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ---- documents ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	docs, err := s.store.ListDocuments(r.Context(), ws(r), store.DocFilter{
		ProjectID:    q.Get("project"),
		IssueID:      q.Get("issue"),
		InitiativeID: q.Get("initiative"),
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(docs))
}

func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDocument(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, d)
}

type docSaveReq struct {
	Title        string   `json:"title"`
	BodyMd       string   `json:"bodyMd"`
	Type         string   `json:"type"`
	ProjectId    *string  `json:"projectId"`
	InitiativeId *string  `json:"initiativeId"`
	IssueId      *string  `json:"issueId"`
	LabelIds     []string `json:"labelIds"`
	LabelNames   []string `json:"labelNames"`
}

func (s *Server) handleSaveDocument(w http.ResponseWriter, r *http.Request) {
	var req docSaveReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	d := models.Document{
		Title:  req.Title,
		BodyMD: req.BodyMd,
		Type:   req.Type,
	}
	if id := r.PathValue("id"); id != "" {
		d.ID = id
		// Preserve type on curate when the client didn't send one.
		if d.Type == "" {
			if cur, err := s.store.GetDocument(r.Context(), ws(r), id); err == nil {
				d.Type = cur.Type
			}
		}
	} else {
		// New docs are attributed to whoever created them (MCP bearer => ai).
		d.Author = auth.ActorFrom(r.Context())
	}
	// Empty string detaches; a value attaches; absent (nil) leaves default.
	if req.ProjectId != nil {
		d.ProjectID = strPtr(*req.ProjectId)
	}
	if req.InitiativeId != nil {
		d.InitiativeID = strPtr(*req.InitiativeId)
	}
	if req.IssueId != nil {
		d.IssueID = strPtr(*req.IssueId)
	}
	saved, err := s.store.SaveDocument(r.Context(), ws(r), d)
	if handleStoreErr(w, err) {
		return
	}
	if req.LabelIds != nil || req.LabelNames != nil {
		if err := s.store.SetDocumentLabels(r.Context(), ws(r), saved.ID, req.LabelIds, req.LabelNames); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		saved, _ = s.store.GetDocument(r.Context(), ws(r), saved.ID)
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteDocument(r.Context(), ws(r), r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
