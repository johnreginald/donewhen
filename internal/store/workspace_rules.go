package store

import (
	"context"
	"encoding/json"
	"strings"
)

// WorkspaceAllowedTools are the rules every agent in the workspace gets, on
// top of the built-in defaults and its own.
func (s *Store) WorkspaceAllowedTools(ctx context.Context, wsID string) ([]string, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT allowed_tools FROM workspaces WHERE id = $1`, wsID).Scan(&raw); err != nil {
		return nil, err
	}
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	return out, nil
}

// SetWorkspaceAllowedTools replaces the workspace's rules, trimmed and
// without duplicates.
func (s *Store) SetWorkspaceAllowedTools(ctx context.Context, wsID string, rules []string) ([]string, error) {
	if err := checkRules(rules); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, r := range rules {
		if r = strings.TrimSpace(r); r != "" && !seen[r] {
			seen[r] = true
			clean = append(clean, r)
		}
	}
	b, _ := json.Marshal(clean)
	if _, err := s.pool.Exec(ctx, `UPDATE workspaces SET allowed_tools = $2 WHERE id = $1`, wsID, b); err != nil {
		return nil, err
	}
	return clean, nil
}
