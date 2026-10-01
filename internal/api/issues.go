package api

import (
	"net/http"
	"strconv"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	issues, err := s.store.ListIssues(r.Context(), store.IssueFilter{
		WorkspaceID:  ws(r),
		StateID:      q.Get("state"),
		ProjectID:    q.Get("project"),
		InitiativeID: q.Get("initiative"),
		LabelID:      q.Get("label"),
		Query:        q.Get("q"),
		ParentKey:    q.Get("parent"),
		Limit:        limit,

		IncludeArchived: q.Get("includeArchived") == "1" || q.Get("includeArchived") == "true",
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(issues))
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, is)
}

// resolveIssue accepts either a UUID or a human key (e.g. K-42).
func (s *Server) resolveIssue(r *http.Request, ref string) (models.Issue, error) {
	// A key looks like "PREFIX-<n>"; a UUID is 36 chars with dashes at 8-13-18-23.
	if looksLikeKey(ref) {
		return s.store.GetIssueByKey(r.Context(), ws(r), ref)
	}
	return s.store.GetIssue(r.Context(), ws(r), ref)
}

func looksLikeKey(ref string) bool {
	if len(ref) == 36 && ref[8] == '-' && ref[13] == '-' && ref[18] == '-' && ref[23] == '-' {
		return false // UUID
	}
	return true
}

type issueCreateReq struct {
	Title         string   `json:"title"`
	DescriptionMd string   `json:"descriptionMd"`
	StateId       string   `json:"stateId"`
	StateName     string   `json:"stateName"`
	ProjectId     string   `json:"projectId"`
	AssigneeId    string   `json:"assigneeId"`
	Priority      int      `json:"priority"`
	ParentKey     string   `json:"parentKey"`
	LabelIds      []string `json:"labelIds"`
	LabelNames    []string `json:"labelNames"`
	Force         bool     `json:"force"` // see issueUpdateReq.Force
}

// canForceGate reports whether this caller may override the done-when gate.
func canForceGate(r *http.Request) bool {
	return !auth.IsBearer(r.Context()) && canAdmin(r)
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
		ParentKey:     strPtr(req.ParentKey),
		LabelIDs:      req.LabelIds,
		LabelNames:    req.LabelNames,
		ForceGate:     req.Force && canForceGate(r),
	}
	is, err := s.svc.CreateIssue(r.Context(), ws(r), in, auth.ActorFrom(r.Context()))
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
	ParentKey     *string  `json:"parentKey"`
	LabelIds      []string `json:"labelIds"`
	LabelNames    []string `json:"labelNames"`
	// Force skips the done-when gate. Honoured only for an owner/admin browser
	// session; a bearer token's value is ignored.
	Force bool `json:"force"`
	// BlockedReason is required when the patch moves the issue to Blocked.
	BlockedReason string `json:"blockedReason"`
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
		ForceGate:     req.Force && canForceGate(r),
		BlockedReason: req.BlockedReason,
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
	if req.ParentKey != nil {
		p.SetParent = true
		p.ParentKey = strPtr(*req.ParentKey)
	}
	if req.LabelIds != nil || req.LabelNames != nil {
		p.ReplaceLabels = true
		p.LabelIDs = req.LabelIds
		p.LabelNames = req.LabelNames
	}
	cur, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	is, err := s.svc.UpdateIssue(r.Context(), ws(r), cur.ID, p, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, is)
}

func (s *Server) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	cur, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	if handleStoreErr(w, s.svc.DeleteIssue(r.Context(), ws(r), cur.ID, auth.ActorFrom(r.Context()))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
