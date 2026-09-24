package api

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	Status         *string  `json:"status"`
}

func (b agentReq) input(raw map[string]json.RawMessage) store.AgentInput {
	_, setAllowed := raw["allowedTools"]
	return store.AgentInput{
		Name: b.Name, Role: b.Role, Harness: b.Harness, Model: b.Model, Effort: b.Effort,
		InstructionsMD: b.InstructionsMd, AllowedTools: b.AllowedTools, SetAllowed: setAllowed,
		MaxTurns: b.MaxTurns, Status: b.Status,
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
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	h, err := s.store.HostHeartbeat(r.Context(), ws(r), body.Name, body.Version, body.Harnesses)
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "host.updated", Host: &h})
	writeJSON(w, 200, h)
}

// ---- jobs ----

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.JobFilter{AgentID: q.Get("agent"), Kind: q.Get("kind")}
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
			resp.Agent = &a
		}
	}
	writeJSON(w, 200, resp)
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
	ref := body.Agent
	if ref == "" && is.AgentID != nil {
		ref = *is.AgentID
	}
	if ref == "" {
		writeErr(w, http.StatusBadRequest, "choose an agent for this ticket first")
		return
	}
	a, err := s.store.GetAgent(r.Context(), ws(r), ref)
	if handleStoreErr(w, err) {
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
