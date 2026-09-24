package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- interactions: what an agent asked of a human on a ticket ----

const interactionCols = `id, issue_id, agent_id, kind, payload, status, response, created_at, resolved_at`

func scanInteraction(row pgx.Row) (models.Interaction, error) {
	var it models.Interaction
	var payload, response []byte
	err := row.Scan(&it.ID, &it.IssueID, &it.AgentID, &it.Kind, &payload, &it.Status, &response, &it.CreatedAt, &it.ResolvedAt)
	it.Payload, it.Response = json.RawMessage(payload), json.RawMessage(response)
	return it, err
}

// maxQuestions keeps a question card readable; Paperclip's onboarding asks four.
const maxQuestions = 8

// NormaliseQuestions checks an agent's questions and gives each an id.
func NormaliseQuestions(qs []models.Question) ([]models.Question, error) {
	if len(qs) == 0 {
		return nil, invalid("ask at least one question")
	}
	if len(qs) > maxQuestions {
		return nil, invalid("ask at most %d questions at once", maxQuestions)
	}
	for i := range qs {
		qs[i].Text = strings.TrimSpace(qs[i].Text)
		if qs[i].Text == "" {
			return nil, invalid("question %d has no text", i+1)
		}
		qs[i].ID = fmt.Sprintf("q%d", i+1)
		opts := qs[i].Options[:0]
		for _, o := range qs[i].Options {
			if o = strings.TrimSpace(o); o != "" {
				opts = append(opts, o)
			}
		}
		qs[i].Options = opts
		if len(opts) == 0 {
			qs[i].AllowOther = true // a question with no options is answered in words
		}
	}
	return qs, nil
}

// CreateInteraction records a question set or a proposal on a ticket.
func (s *Store) CreateInteraction(ctx context.Context, wsID, issueID, agentID, kind string, payload any) (models.Interaction, error) {
	if kind != "questions" && kind != "proposal" {
		return models.Interaction{}, invalid("unknown interaction kind %q", kind)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return models.Interaction{}, err
	}
	it, err := scanInteraction(s.pool.QueryRow(ctx, `
		INSERT INTO interactions (workspace_id, issue_id, agent_id, kind, payload)
		SELECT $1, i.id, (SELECT a.id FROM agents a WHERE a.id::text = $3 AND a.workspace_id = $1), $4, $5
		FROM issues i WHERE i.id = $2 AND i.workspace_id = $1
		RETURNING `+interactionCols, wsID, issueID, agentID, kind, b))
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// ListInteractions lists a ticket's interactions, oldest first, as the thread reads.
func (s *Store) ListInteractions(ctx context.Context, wsID, issueID string) ([]models.Interaction, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+interactionCols+` FROM interactions
		WHERE workspace_id = $1 AND issue_id = $2 ORDER BY created_at`, wsID, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Interaction
	for rows.Next() {
		it, err := scanInteraction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetInteraction reads one interaction.
func (s *Store) GetInteraction(ctx context.Context, wsID, id string) (models.Interaction, error) {
	it, err := scanInteraction(s.pool.QueryRow(ctx,
		`SELECT `+interactionCols+` FROM interactions WHERE workspace_id = $1 AND id::text = $2`, wsID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// ResolveInteraction closes an open interaction with the human's response.
// It resolves once: a second answer is a conflict, not an overwrite.
func (s *Store) ResolveInteraction(ctx context.Context, wsID, id, status string, response any) (models.Interaction, error) {
	switch status {
	case "answered", "approved", "rejected", "canceled":
	default:
		return models.Interaction{}, invalid("cannot resolve an interaction as %q", status)
	}
	b, err := json.Marshal(response)
	if err != nil {
		return models.Interaction{}, err
	}
	it, err := scanInteraction(s.pool.QueryRow(ctx, `
		UPDATE interactions SET status = $3, response = $4, resolved_at = now()
		WHERE workspace_id = $1 AND id::text = $2 AND status = 'open'
		RETURNING `+interactionCols, wsID, id, status, b))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gerr := s.GetInteraction(ctx, wsID, id); gerr != nil {
			return it, gerr
		}
		return it, fmt.Errorf("already resolved: %w", ErrConflict)
	}
	return it, err
}

// CheckAnswers matches answers to the questions they answer: every question
// answered, each choice one of its options, and only one unless it is multi.
func CheckAnswers(qs []models.Question, answers []models.Answer) error {
	byID := map[string]models.Answer{}
	for _, a := range answers {
		byID[a.QuestionID] = a
	}
	for _, q := range qs {
		a, ok := byID[q.ID]
		if !ok || (len(a.Choices) == 0 && strings.TrimSpace(a.Other) == "") {
			return invalid("%q is not answered", q.Text)
		}
		if len(a.Choices) > 1 && !q.Multi {
			return invalid("%q takes one choice", q.Text)
		}
		for _, c := range a.Choices {
			if !contains(q.Options, c) {
				return invalid("%q is not an option for %q", c, q.Text)
			}
		}
		if strings.TrimSpace(a.Other) != "" && !q.AllowOther {
			return invalid("%q does not take a written answer", q.Text)
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---- agent sessions ----

// GetAgentSession reads an agent's session on a ticket, or ErrNotFound.
func (s *Store) GetAgentSession(ctx context.Context, wsID, agentID, issueID string) (models.AgentSession, error) {
	var ss models.AgentSession
	err := s.pool.QueryRow(ctx, `
		SELECT s.agent_id, s.issue_id, s.session_id, s.cwd, s.turns, s.updated_at
		FROM agent_sessions s JOIN agents a ON a.id = s.agent_id
		WHERE a.workspace_id = $1 AND s.agent_id::text = $2 AND s.issue_id::text = $3`,
		wsID, agentID, issueID).Scan(&ss.AgentID, &ss.IssueID, &ss.SessionID, &ss.Cwd, &ss.Turns, &ss.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ss, ErrNotFound
	}
	return ss, err
}

// SaveAgentSession records the session a turn ended in, counting the turn.
func (s *Store) SaveAgentSession(ctx context.Context, wsID, agentID, issueID, sessionID, cwd string) (models.AgentSession, error) {
	if sessionID == "" {
		return models.AgentSession{}, invalid("session id is required")
	}
	var ss models.AgentSession
	err := s.pool.QueryRow(ctx, `
		INSERT INTO agent_sessions (agent_id, issue_id, session_id, cwd, turns)
		SELECT a.id, i.id, $4, $5, 1 FROM agents a, issues i
		WHERE a.id::text = $2 AND i.id::text = $3 AND a.workspace_id = $1 AND i.workspace_id = $1
		ON CONFLICT (agent_id, issue_id) DO UPDATE
		   SET session_id = EXCLUDED.session_id, cwd = EXCLUDED.cwd,
		       turns = agent_sessions.turns + 1, updated_at = now()
		RETURNING agent_id, issue_id, session_id, cwd, turns, updated_at`,
		wsID, agentID, issueID, sessionID, cwd).Scan(&ss.AgentID, &ss.IssueID, &ss.SessionID, &ss.Cwd, &ss.Turns, &ss.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ss, ErrNotFound
	}
	return ss, err
}

// ---- proposals: an agent's split of the work into tickets ----

// ProposedTicket is one ticket an agent proposes.
type ProposedTicket struct {
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Criteria    []ProposedCriterion `json:"criteria"`
}

// ProposedCriterion is a done-when line, typed as a stored criterion is.
type ProposedCriterion struct {
	Text  string          `json:"text"`
	Kind  string          `json:"kind"`
	Check json.RawMessage `json:"check,omitempty"`
}

// Proposal is the payload of a proposal interaction.
type Proposal struct {
	Summary string           `json:"summary,omitempty"`
	Tickets []ProposedTicket `json:"tickets"`
}

// maxProposedTickets keeps a split reviewable in one card.
const maxProposedTickets = 12

// NormaliseProposal checks a proposal the way `orchestrator propose` checks a
// checklist: every ticket must carry at least one criterion a program can
// decide — a command or a policy — or nothing could ever gate it.
func NormaliseProposal(p Proposal) (Proposal, error) {
	if len(p.Tickets) == 0 {
		return p, invalid("propose at least one ticket")
	}
	if len(p.Tickets) > maxProposedTickets {
		return p, invalid("propose at most %d tickets at once", maxProposedTickets)
	}
	for i := range p.Tickets {
		t := &p.Tickets[i]
		t.Title = strings.TrimSpace(t.Title)
		if t.Title == "" {
			return p, invalid("ticket %d has no title", i+1)
		}
		gating := false
		for j := range t.Criteria {
			c := &t.Criteria[j]
			c.Text = strings.TrimSpace(c.Text)
			if c.Kind == "" {
				c.Kind = models.CriterionManual
			}
			if c.Text == "" {
				return p, invalid("%q: criterion %d has no text", t.Title, j+1)
			}
			var check map[string]any
			_ = json.Unmarshal(c.Check, &check)
			switch c.Kind {
			case models.CriterionManual, models.CriterionJudgment:
			case models.CriterionDeterministic:
				if cmd, _ := check["cmd"].(string); strings.TrimSpace(cmd) == "" {
					return p, invalid("%q: %q needs a check {\"cmd\": \"...\"}", t.Title, c.Text)
				}
				gating = true
			case models.CriterionPolicy:
				if pol, _ := check["policy"].(string); strings.TrimSpace(pol) == "" {
					return p, invalid("%q: %q needs a check {\"policy\": \"...\"}", t.Title, c.Text)
				}
				gating = true
			default:
				return p, invalid("%q: unknown criterion kind %q", t.Title, c.Kind)
			}
		}
		if !gating {
			return p, invalid("%q has nothing a program can check: add a command or a policy criterion", t.Title)
		}
	}
	return p, nil
}
