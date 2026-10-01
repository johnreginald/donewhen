package api

import (
	"encoding/json"
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

// ---- comments ----

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	comments, err := s.store.ListComments(r.Context(), ws(r), id)
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
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	c, err := s.svc.AddComment(r.Context(), ws(r), id, body.BodyMd, auth.ActorFrom(r.Context()))
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

type docCreateReq struct {
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
	var req docCreateReq
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
		// New docs are attributed to whoever created them (MCP bearer => ai).
		Author: auth.ActorFrom(r.Context()),
	}
	if req.ProjectId != nil {
		d.ProjectID = strPtr(*req.ProjectId)
	}
	if req.InitiativeId != nil {
		d.InitiativeID = strPtr(*req.InitiativeId)
	}
	if req.IssueId != nil {
		d.IssueID = strPtr(*req.IssueId)
	}
	// The document and its labels commit together.
	saved, err := s.store.CreateDocument(r.Context(), ws(r), d, store.DocLabels{
		Set: req.LabelIds != nil || req.LabelNames != nil, IDs: req.LabelIds, Names: req.LabelNames,
	})
	if handleStoreErr(w, err) {
		return
	}
	// Live: every connected client refreshes its document list on this.
	s.bus.Publish(events.Event{Type: events.DocumentSaved, WorkspaceID: ws(r), Actor: saved.Author, Document: &saved})
	writeJSON(w, 200, saved)
}

// handleUpdateDocument is PATCH: only the fields present in the body change. A
// null or empty projectId/initiativeId/issueId detaches that link; absent keeps it.
func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title        *string         `json:"title"`
		BodyMd       *string         `json:"bodyMd"`
		Type         *string         `json:"type"`
		ProjectId    json.RawMessage `json:"projectId"`
		InitiativeId json.RawMessage `json:"initiativeId"`
		IssueId      json.RawMessage `json:"issueId"`
		LabelIds     []string        `json:"labelIds"`
		LabelNames   []string        `json:"labelNames"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title != nil && *req.Title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	var ids [3]*string
	for i, raw := range []json.RawMessage{req.ProjectId, req.InitiativeId, req.IssueId} {
		v, ok := patchString(raw)
		if !ok {
			writeErr(w, http.StatusBadRequest, "projectId, initiativeId and issueId must be strings or null")
			return
		}
		ids[i] = v
	}
	saved, err := s.store.UpdateDocument(r.Context(), ws(r), r.PathValue("id"), store.DocumentPatch{
		Title: req.Title, BodyMD: req.BodyMd, Type: req.Type,
		ProjectID: ids[0], InitiativeID: ids[1], IssueID: ids[2],
		Labels: store.DocLabels{Set: req.LabelIds != nil || req.LabelNames != nil, IDs: req.LabelIds, Names: req.LabelNames},
	})
	if handleStoreErr(w, err) {
		return
	}
	s.bus.Publish(events.Event{Type: events.DocumentSaved, WorkspaceID: ws(r), Actor: saved.Author, Document: &saved})
	writeJSON(w, 200, saved)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if handleStoreErr(w, s.store.DeleteDocument(r.Context(), ws(r), id)) {
		return
	}
	s.bus.Publish(events.Event{Type: events.DocumentDeleted, WorkspaceID: ws(r), DocumentID: id})
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
