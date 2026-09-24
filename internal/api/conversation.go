package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

// handleAskAgent is "Ask <agent>" on a ticket: post the human's message, if
// any, and queue the agent's next turn in the conversation.
func (s *Server) handleAskAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Agent   string `json:"agent"`
		Message string `json:"message"`
	}
	_ = readJSON(r, &body)
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	a, ok := s.agentFor(w, r, is, body.Agent)
	if !ok {
		return
	}
	if msg := strings.TrimSpace(body.Message); msg != "" {
		if _, err := s.svc.AddComment(r.Context(), ws(r), is.ID, msg, auth.ActorFrom(r.Context()), ""); handleStoreErr(w, err) {
			return
		}
	}
	s.queueTurn(w, r, is, a, map[string]any{"reason": "message"}, http.StatusCreated)
}

// agentFor resolves the agent a request names, or else the ticket's own.
func (s *Server) agentFor(w http.ResponseWriter, r *http.Request, is models.Issue, ref string) (models.Agent, bool) {
	if ref == "" && is.AgentID != nil {
		ref = *is.AgentID
	}
	if ref == "" {
		writeErr(w, http.StatusBadRequest, "choose an agent for this ticket first")
		return models.Agent{}, false
	}
	a, err := s.store.GetAgent(r.Context(), ws(r), ref)
	if handleStoreErr(w, err) {
		return a, false
	}
	if a.Status == "paused" {
		writeErr(w, http.StatusConflict, a.Name+" is paused")
		return a, false
	}
	return a, true
}

// queueTurn queues a chat turn and answers with the job.
func (s *Server) queueTurn(w http.ResponseWriter, r *http.Request, is models.Issue, a models.Agent, input any, status int) {
	j, err := s.store.EnqueueJob(r.Context(), ws(r), store.JobInput{Kind: "chat", AgentID: a.ID, IssueID: is.ID, Input: input})
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "job.updated", Job: &j, IssueID: is.ID})
	writeJSON(w, status, j)
}

func (s *Server) handleListInteractions(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	list, err := s.store.ListInteractions(r.Context(), ws(r), id)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(list))
}

// handleRespondInteraction answers a question card. Answering wakes the
// agent that asked: its next turn is queued with the answers.
func (s *Server) handleRespondInteraction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Answers  []models.Answer `json:"answers"`
		Decision string          `json:"decision"` // approve | reject — proposals
		Note     string          `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	it, err := s.store.GetInteraction(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	var resolved models.Interaction
	switch it.Kind {
	case "questions":
		var p struct {
			Questions []models.Question `json:"questions"`
		}
		_ = json.Unmarshal(it.Payload, &p)
		if handleStoreErr(w, store.CheckAnswers(p.Questions, body.Answers)) {
			return
		}
		resolved, err = s.store.ResolveInteraction(r.Context(), ws(r), it.ID, "answered", map[string]any{"answers": body.Answers})
	default:
		resolved, err = s.resolveProposal(r, it, body.Decision, body.Note)
	}
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "interaction.updated", Interaction: &resolved, IssueID: resolved.IssueID})

	// Wake the agent that asked.
	if resolved.AgentID != nil {
		is, ierr := s.store.GetIssue(r.Context(), ws(r), resolved.IssueID)
		a, aerr := s.store.GetAgent(r.Context(), ws(r), *resolved.AgentID)
		if ierr == nil && aerr == nil && a.Status == "active" {
			j, jerr := s.store.EnqueueJob(r.Context(), ws(r), store.JobInput{
				Kind: "chat", AgentID: a.ID, IssueID: is.ID,
				Input: map[string]any{"reason": "response", "interactionId": resolved.ID},
			})
			if jerr == nil {
				s.publish(r, events.Event{Type: "job.updated", Job: &j, IssueID: is.ID})
			}
		}
	}
	writeJSON(w, 200, resolved)
}

func (s *Server) handleGetAgentSession(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.GetAgent(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	is, err := s.resolveIssue(r, r.PathValue("issue"))
	if handleStoreErr(w, err) {
		return
	}
	ss, err := s.store.GetAgentSession(r.Context(), ws(r), a.ID, is.ID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, ss)
}

func (s *Server) handleSaveAgentSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID   string `json:"sessionId"`
		Cwd         string `json:"cwd"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	a, err := s.store.GetAgent(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	is, err := s.resolveIssue(r, r.PathValue("issue"))
	if handleStoreErr(w, err) {
		return
	}
	ss, err := s.store.SaveAgentSession(r.Context(), ws(r), a.ID, is.ID, body.SessionID, body.Cwd, body.Fingerprint)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, ss)
}

// resolveProposal approves or rejects a proposal. Approving creates each
// proposed ticket under the one it was discussed on, in Ready, for the same
// agent, with its typed done-when — and keeps the parent's epic and repo, so
// the new tickets are worked in the right place. Running them is still a Run.
func (s *Server) resolveProposal(r *http.Request, it models.Interaction, decision, note string) (models.Interaction, error) {
	switch decision {
	case "reject":
		return s.store.ResolveInteraction(r.Context(), ws(r), it.ID, "rejected", map[string]any{"note": note})
	case "approve":
	default:
		return models.Interaction{}, errInvalid("decision must be approve or reject")
	}
	if it.Status != "open" {
		return models.Interaction{}, fmt.Errorf("already %s: %w", it.Status, store.ErrConflict)
	}
	var p store.Proposal
	if err := json.Unmarshal(it.Payload, &p); err != nil {
		return models.Interaction{}, err
	}
	parent, err := s.store.GetIssue(r.Context(), ws(r), it.IssueID)
	if err != nil {
		return models.Interaction{}, err
	}
	var repoLabels []string
	if groups, err := s.store.ListLabelGroups(r.Context(), ws(r)); err == nil {
		for _, g := range groups {
			if g.Name != "repo" {
				continue
			}
			for _, l := range parent.Labels {
				if l.GroupID != nil && *l.GroupID == g.ID {
					repoLabels = append(repoLabels, l.Name)
				}
			}
		}
	}
	parentKey := parent.Key
	var created []string
	for _, t := range p.Tickets {
		is, err := s.svc.CreateIssue(r.Context(), ws(r), store.IssueInput{
			Title:         t.Title,
			DescriptionMD: t.Description,
			StateName:     "Ready",
			ProjectID:     parent.ProjectID,
			ParentKey:     &parentKey,
			AgentID:       it.AgentID,
			Priority:      parent.Priority,
			LabelNames:    repoLabels,
		}, auth.ActorFrom(r.Context()))
		if err != nil {
			return models.Interaction{}, fmt.Errorf("create %q: %w", t.Title, err)
		}
		for _, c := range t.Criteria {
			if _, err := s.store.AddCriterion(r.Context(), ws(r), is.ID, c.Text, c.Kind, c.Check); err != nil {
				return models.Interaction{}, fmt.Errorf("add a criterion to %s: %w", is.Key, err)
			}
		}
		created = append(created, is.Key)
	}
	return s.store.ResolveInteraction(r.Context(), ws(r), it.ID, "approved", map[string]any{"created": created, "note": note})
}

// handleReviewIssue queues Verify or Finish on a handed-back ticket for the
// host that has its worktree — the dashboard's `orchestrator verify` and
// `orchestrator finish`.
func (s *Server) handleReviewIssue(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		is, err := s.resolveIssue(r, r.PathValue("id"))
		if handleStoreErr(w, err) {
			return
		}
		a, ok := s.agentFor(w, r, is, "")
		if !ok {
			return
		}
		j, err := s.store.EnqueueJob(r.Context(), ws(r), store.JobInput{Kind: kind, AgentID: a.ID, IssueID: is.ID})
		if handleStoreErr(w, err) {
			return
		}
		s.publish(r, events.Event{Type: "job.updated", Job: &j})
		writeJSON(w, http.StatusCreated, j)
	}
}
