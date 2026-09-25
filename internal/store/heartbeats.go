package store

import (
	"context"
	"time"
)

// DueHeartbeat is an agent whose timer came round, with its workspace.
type DueHeartbeat struct {
	WorkspaceID string
	AgentID     string
}

// ClaimDueHeartbeats takes the active agents whose heartbeat is due.
func (s *Store) ClaimDueHeartbeats(ctx context.Context, now time.Time) ([]DueHeartbeat, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE agents SET last_heartbeat_at = $1
		WHERE id IN (
			SELECT id FROM agents
			WHERE status = 'active' AND heartbeat_minutes > 0
			  AND (last_heartbeat_at IS NULL OR last_heartbeat_at + make_interval(mins => heartbeat_minutes) <= $1)
			FOR UPDATE SKIP LOCKED
		)
		RETURNING workspace_id::text, id::text`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueHeartbeat
	for rows.Next() {
		var d DueHeartbeat
		if err := rows.Scan(&d.WorkspaceID, &d.AgentID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// WaitingForAgent lists the agent's open tickets where a person has written
// since the agent last replied, and no turn is already queued or running.
func (s *Store) WaitingForAgent(ctx context.Context, wsID, agentID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text FROM issues i JOIN workflow_states ws ON ws.id = i.state_id
		WHERE i.workspace_id = $1 AND i.agent_id::text = $2
		  AND ws.category NOT IN ('completed', 'canceled')
		  AND EXISTS (
			SELECT 1 FROM comments c WHERE c.issue_id = i.id AND c.agent_id IS NULL AND c.actor = 'human'
			  AND c.created_at > coalesce((SELECT max(a.created_at) FROM comments a
			                               WHERE a.issue_id = i.id AND a.agent_id::text = $2), '-infinity'))
		  AND NOT EXISTS (
			SELECT 1 FROM jobs j WHERE j.issue_id = i.id AND j.kind = 'chat' AND j.status IN ('queued', 'claimed'))`,
		wsID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
