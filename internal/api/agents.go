package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

// ---- agents ----

type agentReq struct {
	Name           *string  `json:"name"`
	Role           *string  `json:"role"`
	Harness        *string  `json:"harness"`
	Model          *string  `json:"model"`
	Effort         *string  `json:"effort"`
	InstructionsMd *string  `json:"instructionsMd"`
	AllowedTools   []string `json:"allowedTools"`
	MaxTurns       *int     `json:"maxTurns"`
	RunInTerminal  *bool    `json:"runInTerminal"`
	Heartbeat      *int     `json:"heartbeatMinutes"`
	BudgetTokens   *int64   `json:"budgetTokens"`
	BudgetUSD      *float64 `json:"budgetUsd"`
	Status         *string  `json:"status"`
}

func (b agentReq) input(raw map[string]json.RawMessage) store.AgentInput {
	_, setAllowed := raw["allowedTools"]
	return store.AgentInput{
		Name: b.Name, Role: b.Role, Harness: b.Harness, Model: b.Model, Effort: b.Effort,
		InstructionsMD: b.InstructionsMd, AllowedTools: b.AllowedTools, SetAllowed: setAllowed,
		MaxTurns: b.MaxTurns, RunInTerminal: b.RunInTerminal, Heartbeat: b.Heartbeat, BudgetTokens: b.BudgetTokens, BudgetUSD: b.BudgetUSD, Status: b.Status,
	}
}

// readAgentReq reads the body twice over: once typed, once raw, so a field
// sent as [] can be told apart from a field not sent.
func readAgentReq(r *http.Request) (agentReq, map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		return agentReq{}, nil, err
	}
	b, _ := json.Marshal(raw)
	var req agentReq
	err := json.Unmarshal(b, &req)
	return req, raw, err
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListAgents(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(list))
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAgent(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	req, raw, err := readAgentReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	a, err := s.store.CreateAgent(r.Context(), ws(r), req.input(raw))
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "agent.saved", Agent: &a})
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	req, raw, err := readAgentReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	a, err := s.store.UpdateAgent(r.Context(), ws(r), r.PathValue("id"), req.input(raw))
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "agent.saved", Agent: &a})
	writeJSON(w, 200, a)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteAgent(r.Context(), ws(r), r.PathValue("id"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAgentRuns(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAgent(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.store.ListRuns(r.Context(), ws(r), store.RunFilter{AgentID: a.ID, Limit: limit})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(runs))
}

// handleTestAgent queues an environment test: a runner host proves the
// agent's harness is installed, signed in, and answers with its model.
func (s *Server) handleTestAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAgent(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	j, err := s.store.EnqueueJob(r.Context(), ws(r), store.JobInput{Kind: "test_env", AgentID: a.ID})
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "job.updated", Job: &j})
	writeJSON(w, http.StatusCreated, j)
}

// ---- runner hosts ----

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.ListHosts(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(hosts))
}

func (s *Server) handleHostHeartbeat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string                 `json:"name"`
		Version   string                 `json:"version"`
		Harnesses []models.HarnessStatus `json:"harnesses"`
		// Started is set on a host's first heartbeat: whatever it had
		// claimed before it restarted will never be finished.
		Started bool `json:"started"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	h, err := s.store.HostHeartbeat(r.Context(), ws(r), body.Name, body.Version, body.Harnesses)
	if handleStoreErr(w, err) {
		return
	}
	if body.Started {
		jobs, runs, err := s.store.ReleaseHost(r.Context(), ws(r), body.Name, "the host restarted before finishing")
		if handleStoreErr(w, err) {
			return
		}
		s.publishReaped(ws(r), jobs, runs)
	}
	s.publish(r, events.Event{Type: "host.updated", Host: &h})
	writeJSON(w, 200, h)
}

// ---- jobs ----

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.JobFilter{AgentID: q.Get("agent"), Kind: q.Get("kind"), Active: q.Get("active") != ""}
	if ref := q.Get("issue"); ref != "" {
		is, err := s.resolveIssue(r, ref)
		if handleStoreErr(w, err) {
			return
		}
		f.IssueID = is.ID
	}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	jobs, err := s.store.ListJobs(r.Context(), ws(r), f)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(jobs))
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.store.GetJob(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, j)
}

func (s *Server) handleEnqueueJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind  string `json:"kind"`
		Agent string `json:"agent"` // id or slug
		Issue string `json:"issue"` // id or key
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	a, err := s.store.GetAgent(r.Context(), ws(r), body.Agent)
	if handleStoreErr(w, err) {
		return
	}
	in := store.JobInput{Kind: body.Kind, AgentID: a.ID}
	if body.Issue != "" {
		is, err := s.resolveIssue(r, body.Issue)
		if handleStoreErr(w, err) {
			return
		}
		in.IssueID = is.ID
	}
	j, err := s.store.EnqueueJob(r.Context(), ws(r), in)
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "job.updated", Job: &j})
	writeJSON(w, http.StatusCreated, j)
}

// handleClaimJob gives a runner host the next job it can run, or 204.
func (s *Server) handleClaimJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host      string   `json:"host"`
		Harnesses []string `json:"harnesses"`
	}
	if err := readJSON(r, &body); err != nil || body.Host == "" {
		writeErr(w, http.StatusBadRequest, "host required")
		return
	}
	j, ok, err := s.store.ClaimJob(r.Context(), ws(r), body.Host, body.Harnesses)
	if handleStoreErr(w, err) {
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.publish(r, events.Event{Type: "job.updated", Job: &j})
	resp := struct {
		models.Job
		Agent *models.Agent `json:"agent,omitempty"`
	}{Job: j}
	if j.AgentID != nil {
		if a, err := s.store.GetAgent(r.Context(), ws(r), *j.AgentID); err == nil {
			// The host runs with the agent's rules and the workspace's.
			if shared, err := s.store.WorkspaceAllowedTools(r.Context(), ws(r)); err == nil {
				a.AllowedTools = append(append([]string{}, shared...), a.AllowedTools...)
			}
			resp.Agent = &a
		}
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleGetWorkspaceAllowed(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.WorkspaceAllowedTools(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowedTools": rules})
}

// handleSetWorkspaceAllowed replaces the commands every agent in the
// workspace may run without asking.
func (s *Server) handleSetWorkspaceAllowed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AllowedTools []string `json:"allowedTools"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rules, err := s.store.SetWorkspaceAllowedTools(r.Context(), ws(r), body.AllowedTools)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowedTools": rules})
}

func (s *Server) handleFinishJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Host   string          `json:"host"`
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := readJSON(r, &body); err != nil || body.Host == "" {
		writeErr(w, http.StatusBadRequest, "host required")
		return
	}
	j, err := s.store.FinishJob(r.Context(), ws(r), r.PathValue("id"), body.Host, body.Status, body.Result, body.Error)
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "job.updated", Job: &j})
	writeJSON(w, 200, j)
}

// publish sends an event to this workspace's live stream.
func (s *Server) publish(r *http.Request, e events.Event) {
	e.WorkspaceID = ws(r)
	// Every event about a ticket names it, so a page following one ticket
	// can pick out its own events.
	if e.IssueID == "" {
		switch {
		case e.Job != nil && e.Job.IssueID != nil:
			e.IssueID = *e.Job.IssueID
		case e.Run != nil && e.Run.IssueID != nil:
			e.IssueID = *e.Run.IssueID
		case e.Interaction != nil:
			e.IssueID = e.Interaction.IssueID
		}
	}
	if e.Actor == "" {
		e.Actor = auth.ActorFrom(r.Context())
	}
	s.bus.Publish(e)
}

// handleRunIssue queues a ticket for an agent: the one named in the body, or
// else the agent the ticket is for. A runner host picks it up.
func (s *Server) handleRunIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Agent string `json:"agent"`
	}
	_ = readJSON(r, &body)
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	// A ticket waits for the tickets that block it: its work builds on
	// theirs, which is only on the base branch once they are Done.
	open, err := s.store.OpenBlockers(r.Context(), ws(r), is.ID)
	if handleStoreErr(w, err) {
		return
	}
	if len(open) > 0 {
		writeErr(w, http.StatusConflict, store.BlockedMessage(open))
		return
	}
	a, ok := s.agentFor(w, r, is, body.Agent)
	if !ok {
		return
	}
	if a.Status == "paused" {
		writeErr(w, http.StatusConflict, a.Name+" is paused")
		return
	}
	j, err := s.store.EnqueueJob(r.Context(), ws(r), store.JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is.ID})
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "job.updated", Job: &j, IssueID: is.ID})
	writeJSON(w, http.StatusCreated, j)
}

// publishReaped announces work the store closed because its host went away.
func (s *Server) publishReaped(wsID string, jobs []models.Job, runs []models.Run) {
	for i := range jobs {
		e := events.Event{Type: "job.updated", WorkspaceID: wsID, Actor: auth.ActorAI, Job: &jobs[i]}
		if jobs[i].IssueID != nil {
			e.IssueID = *jobs[i].IssueID
		}
		s.bus.Publish(e)
	}
	for i := range runs {
		e := events.Event{Type: "run.finished", WorkspaceID: wsID, Actor: auth.ActorAI, Run: &runs[i]}
		if runs[i].IssueID != nil {
			e.IssueID = *runs[i].IssueID
		}
		s.bus.Publish(e)
	}
}

// Reap closes work whose machine has gone away, across every workspace, and
// announces it. The server calls it every minute.
func (s *Server) Reap(ctx context.Context) {
	byWS, err := s.store.ReapStale(ctx, 5*time.Minute, 3*time.Hour)
	if err != nil {
		log.Printf("reap: %v", err)
		return
	}
	for wsID, r := range byWS {
		s.publishReaped(wsID, r.Jobs, r.Runs)
	}
}

func (s *Server) handleListPriorityAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPriorityAgents(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleSetPriorityAgent sets (or, with no agent, clears) the default agent
// for one priority.
func (s *Server) handleSetPriorityAgent(w http.ResponseWriter, r *http.Request) {
	var body store.PriorityAgent
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if handleStoreErr(w, s.store.SetPriorityAgent(r.Context(), ws(r), body.Priority, body.AgentID)) {
		return
	}
	s.handleListPriorityAgents(w, r)
}

// handleStopJob stops one job: canceled now if still queued, or ended by its
// host within a few seconds if one is working on it.
func (s *Server) handleStopJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.store.StopJob(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	e := events.Event{Type: "job.updated", Job: &j}
	if j.IssueID != nil {
		e.IssueID = *j.IssueID
	}
	s.publish(r, e)
	writeJSON(w, http.StatusOK, j)
}

// handleStopIssue stops everything an agent is doing or about to do on a
// ticket — the Stop button on the task.
func (s *Server) handleStopIssue(w http.ResponseWriter, r *http.Request) {
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	ids, err := s.store.ActiveJobsFor(r.Context(), ws(r), is.ID)
	if handleStoreErr(w, err) {
		return
	}
	stopped := []models.Job{}
	for _, id := range ids {
		j, err := s.store.StopJob(r.Context(), ws(r), id)
		if err != nil {
			continue
		}
		stopped = append(stopped, j)
		s.publish(r, events.Event{Type: "job.updated", Job: &j, IssueID: is.ID})
	}
	writeJSON(w, http.StatusOK, stopped)
}
