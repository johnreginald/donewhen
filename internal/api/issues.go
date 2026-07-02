package api

import (
	"net/http"
	"strconv"

	"kanri/internal/auth"
	"kanri/internal/store"
)

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	issues, err := s.store.ListIssues(r.Context(), store.IssueFilter{
		StateID:   q.Get("state"),
		ProjectID: q.Get("project"),
		Query:     q.Get("q"),
		Limit:     limit,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(issues))
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	is, err := s.store.GetIssue(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, is)
}

type issueCreateReq struct {
	Title         string   `json:"title"`
	DescriptionMd string   `json:"descriptionMd"`
	StateId       string   `json:"stateId"`
	StateName     string   `json:"stateName"`
	ProjectId     string   `json:"projectId"`
	AssigneeId    string   `json:"assigneeId"`
	Priority      int      `json:"priority"`
	LabelIds      []string `json:"labelIds"`
	LabelNames    []string `json:"labelNames"`
}

func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	var req issueCreateReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	in := store.IssueInput{
		Title:         req.Title,
		DescriptionMD: req.DescriptionMd,
		StateID:       req.StateId,
		StateName:     req.StateName,
		ProjectID:     strPtr(req.ProjectId),
		AssigneeID:    strPtr(req.AssigneeId),
		Priority:      req.Priority,
		LabelIDs:      req.LabelIds,
		LabelNames:    req.LabelNames,
	}
	is, err := s.svc.CreateIssue(r.Context(), in, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, is)
}

type issueUpdateReq struct {
	Title         *string  `json:"title"`
	DescriptionMd *string  `json:"descriptionMd"`
	StateId       *string  `json:"stateId"`
	StateName     *string  `json:"stateName"`
	ProjectId     *string  `json:"projectId"`
	AssigneeId    *string  `json:"assigneeId"`
	Priority      *int     `json:"priority"`
	Position      *float64 `json:"position"`
	LabelIds      []string `json:"labelIds"`
	LabelNames    []string `json:"labelNames"`
}

func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	var req issueUpdateReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	p := store.IssuePatch{
		Title:         req.Title,
		DescriptionMD: req.DescriptionMd,
		StateID:       req.StateId,
		StateName:     req.StateName,
		Priority:      req.Priority,
		Position:      req.Position,
	}
	// Empty string clears the relation; a value sets it; absent leaves unchanged.
	if req.ProjectId != nil {
		p.SetProject = true
		p.ProjectID = strPtr(*req.ProjectId)
	}
	if req.AssigneeId != nil {
		p.SetAssignee = true
		p.AssigneeID = strPtr(*req.AssigneeId)
	}
	if req.LabelIds != nil || req.LabelNames != nil {
		p.ReplaceLabels = true
		p.LabelIDs = req.LabelIds
		p.LabelNames = req.LabelNames
	}
	is, err := s.svc.UpdateIssue(r.Context(), r.PathValue("id"), p, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, is)
}

func (s *Server) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.svc.DeleteIssue(r.Context(), r.PathValue("id"), auth.ActorFrom(r.Context()))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
