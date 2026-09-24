package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

type routineReq struct {
	Name          *string                   `json:"name"`
	AgentID       *string                   `json:"agentId"`
	ProjectID     *string                   `json:"projectId"`
	Title         *string                   `json:"title"`
	DescriptionMd *string                   `json:"descriptionMd"`
	Criteria      []store.ProposedCriterion `json:"criteria"`
	Labels        []string                  `json:"labels"`
	Schedule      *string                   `json:"schedule"`
	Timezone      *string                   `json:"timezone"`
	Enabled       *bool                     `json:"enabled"`
	AutoRun       *bool                     `json:"autoRun"`
}

func readRoutineReq(r *http.Request) (store.RoutineInput, error) {
	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		return store.RoutineInput{}, err
	}
	b, _ := json.Marshal(raw)
	var q routineReq
	if err := json.Unmarshal(b, &q); err != nil {
		return store.RoutineInput{}, err
	}
	_, setAgent := raw["agentId"]
	_, setProject := raw["projectId"]
	_, setCriteria := raw["criteria"]
	_, setLabels := raw["labels"]
	return store.RoutineInput{
		Name: q.Name, AgentID: q.AgentID, SetAgent: setAgent, ProjectID: q.ProjectID, SetProject: setProject,
		Title: q.Title, DescriptionMD: q.DescriptionMd, Criteria: q.Criteria, SetCriteria: setCriteria,
		Labels: q.Labels, SetLabels: setLabels,
		Schedule: q.Schedule, Timezone: q.Timezone, Enabled: q.Enabled, AutoRun: q.AutoRun,
	}, nil
}

func (s *Server) handleListRoutines(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListRoutines(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(list))
}

func (s *Server) handleCreateRoutine(w http.ResponseWriter, r *http.Request) {
	in, err := readRoutineReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	rt, err := s.store.CreateRoutine(r.Context(), ws(r), in)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, rt)
}

func (s *Server) handleUpdateRoutine(w http.ResponseWriter, r *http.Request) {
	in, err := readRoutineReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	rt, err := s.store.UpdateRoutine(r.Context(), ws(r), r.PathValue("id"), in)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, rt)
}

func (s *Server) handleDeleteRoutine(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteRoutine(r.Context(), ws(r), r.PathValue("id"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleFireRoutine is "Run now": make the routine's ticket at once.
func (s *Server) handleFireRoutine(w http.ResponseWriter, r *http.Request) {
	rt, err := s.store.GetRoutine(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	is, skipped, err := s.fireRoutine(r.Context(), ws(r), rt, time.Now(), auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	if skipped {
		writeErr(w, http.StatusConflict, "its last ticket is still open")
		return
	}
	writeJSON(w, http.StatusCreated, is)
}

// fireRoutine makes a routine's ticket — unless the last one is still open,
// Paperclip's "one open routine ticket at a time" — and queues the agent's
// run when the routine asks for it.
func (s *Server) fireRoutine(ctx context.Context, wsID string, rt models.Routine, at time.Time, actor string) (models.Issue, bool, error) {
	if open, err := s.store.RoutineStillOpen(ctx, rt); err != nil || open {
		return models.Issue{}, open, err
	}
	is, err := s.svc.CreateIssue(ctx, wsID, store.IssueInput{
		Title:         store.RoutineTitle(rt, at),
		DescriptionMD: rt.DescriptionMD,
		StateName:     "Ready",
		ProjectID:     rt.ProjectID,
		AgentID:       rt.AgentID,
		LabelNames:    rt.Labels,
	}, actor)
	if err != nil {
		return is, false, err
	}
	var crit []store.ProposedCriterion
	_ = json.Unmarshal(rt.Criteria, &crit)
	for _, c := range crit {
		if _, err := s.store.AddCriterion(ctx, wsID, is.ID, c.Text, c.Kind, c.Check); err != nil {
			return is, false, err
		}
	}
	if err := s.store.RoutineFired(ctx, wsID, rt.ID, is.ID); err != nil {
		return is, false, err
	}
	_ = s.store.RecordActivity(ctx, wsID, models.Activity{
		IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: auth.ActorAI,
		Kind: "routine", Detail: "made by routine " + rt.Name,
	})
	if rt.AutoRun && rt.AgentID != nil {
		j, err := s.store.EnqueueJob(ctx, wsID, store.JobInput{Kind: "run_ticket", AgentID: *rt.AgentID, IssueID: is.ID})
		if err != nil {
			return is, false, err
		}
		s.bus.Publish(events.Event{Type: "job.updated", WorkspaceID: wsID, Actor: auth.ActorAI, Job: &j, IssueID: is.ID})
	}
	return is, false, nil
}

// Tick runs what is due each minute: routines, and agents' heartbeats.
func (s *Server) Tick(ctx context.Context) {
	now := time.Now()
	due, err := s.store.ClaimDueRoutines(ctx, now)
	if err != nil {
		log.Printf("routines: %v", err)
	}
	for _, d := range due {
		if _, skipped, err := s.fireRoutine(ctx, d.WorkspaceID, d.Routine, now, auth.ActorAI); err != nil {
			log.Printf("routine %s: %v", d.Name, err)
		} else if skipped {
			log.Printf("routine %s: skipped, its last ticket is still open", d.Name)
		}
	}

	beats, err := s.store.ClaimDueHeartbeats(ctx, now)
	if err != nil {
		log.Printf("heartbeats: %v", err)
	}
	for _, b := range beats {
		issues, err := s.store.WaitingForAgent(ctx, b.WorkspaceID, b.AgentID)
		if err != nil {
			log.Printf("heartbeat %s: %v", b.AgentID, err)
			continue
		}
		for _, issueID := range issues {
			j, err := s.store.EnqueueJob(ctx, b.WorkspaceID, store.JobInput{
				Kind: "chat", AgentID: b.AgentID, IssueID: issueID, Input: map[string]any{"reason": "message"},
			})
			if err != nil {
				log.Printf("heartbeat %s: %v", b.AgentID, err)
				continue
			}
			s.bus.Publish(events.Event{Type: "job.updated", WorkspaceID: b.WorkspaceID, Actor: auth.ActorAI, Job: &j, IssueID: issueID})
		}
	}
}
