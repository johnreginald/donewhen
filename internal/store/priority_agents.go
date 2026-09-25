package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// PriorityAgent is the agent a task of one priority gets when it has none.
type PriorityAgent struct {
	Priority int    `json:"priority"`
	AgentID  string `json:"agentId"`
}

// ListPriorityAgents lists a workspace's default agent for each priority that
// has one.
func (s *Store) ListPriorityAgents(ctx context.Context, wsID string) ([]PriorityAgent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT priority, agent_id FROM priority_agents WHERE workspace_id = $1 ORDER BY priority`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PriorityAgent{}
	for rows.Next() {
		var p PriorityAgent
		if err := rows.Scan(&p.Priority, &p.AgentID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPriorityAgent sets the default agent for a priority; an empty agent
// clears it. The agent must belong to the workspace.
func (s *Store) SetPriorityAgent(ctx context.Context, wsID string, priority int, agentRef string) error {
	if priority < 0 || priority > 4 {
		return invalid("priority is 0 (none) to 4 (low)")
	}
	if agentRef == "" {
		_, err := s.pool.Exec(ctx, `DELETE FROM priority_agents WHERE workspace_id = $1 AND priority = $2`, wsID, priority)
		return err
	}
	a, err := s.GetAgent(ctx, wsID, agentRef)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO priority_agents (workspace_id, priority, agent_id) VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, priority) DO UPDATE SET agent_id = EXCLUDED.agent_id`, wsID, priority, a.ID)
	return err
}

// IssueAgentRef is the agent that works a task: the one chosen on it, or
// else the workspace's default for its priority. Empty means neither.
func (s *Store) IssueAgentRef(ctx context.Context, wsID string, is models.Issue) (string, error) {
	if is.AgentID != nil && *is.AgentID != "" {
		return *is.AgentID, nil
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT agent_id FROM priority_agents WHERE workspace_id = $1 AND priority = $2`, wsID, is.Priority).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return id, nil
}
