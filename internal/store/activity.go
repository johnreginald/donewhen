package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/johnreginald/donewhen/internal/models"
)

const activityCols = `a.id, a.issue_id, coalesce(a.issue_key,''), coalesce(a.issue_title,''),
	a.actor, a.kind, coalesce(a.field,''), coalesce(a.from_val,''), coalesce(a.to_val,''),
	coalesce(a.detail,''), a.created_at`

func scanActivity(row pgx.Row) (models.Activity, error) {
	var a models.Activity
	err := row.Scan(&a.ID, &a.IssueID, &a.IssueKey, &a.IssueTitle, &a.Actor, &a.Kind,
		&a.Field, &a.FromVal, &a.ToVal, &a.Detail, &a.CreatedAt)
	return a, err
}

func collectActivity(rows pgx.Rows) ([]models.Activity, error) {
	defer rows.Close()
	var out []models.Activity
	for rows.Next() {
		act, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, act)
	}
	return out, rows.Err()
}

// RecordActivity appends one entry to the timeline. Best-effort — callers log
// but don't fail their operation on an activity write error. The workspace is
// stamped on the row so the log survives the issue being deleted.
func (s *Store) RecordActivity(ctx context.Context, wsID string, a models.Activity) error {
	actor := a.Actor
	if actor == "" {
		actor = "human"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO activity (workspace_id, issue_id, issue_key, issue_title, actor, kind, field, from_val, to_val, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		wsID, a.IssueID, a.IssueKey, a.IssueTitle, actor, a.Kind, a.Field, a.FromVal, a.ToVal, a.Detail)
	return err
}

// ListActivity returns an issue's timeline, newest first.
func (s *Store) ListActivity(ctx context.Context, wsID, issueID string) ([]models.Activity, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+activityCols+` FROM activity a
		 WHERE a.issue_id=$1 AND a.workspace_id=$2 ORDER BY a.created_at DESC`, issueID, wsID)
	if err != nil {
		return nil, err
	}
	return collectActivity(rows)
}

// ActivityFilter scopes the recent-activity (log) feed.
type ActivityFilter struct {
	// One of WorkspaceID / WorkspaceIDs is required — the tenancy boundary.
	WorkspaceID  string
	WorkspaceIDs []string
	InitiativeID string
	ProjectID    string
	Actor        string // human | ai
	Limit        int
}

// ListRecentActivity returns the cross-issue log feed, newest first.
func (s *Store) ListRecentActivity(ctx context.Context, f ActivityFilter) ([]models.Activity, error) {
	if f.WorkspaceID == "" && len(f.WorkspaceIDs) == 0 {
		return nil, fmt.Errorf("ListRecentActivity: workspace id is required")
	}
	q := `SELECT ` + activityCols + ` FROM activity a WHERE a.workspace_id = ANY($1)`
	args := []any{scopeIDs(f.WorkspaceID, f.WorkspaceIDs)}
	n := 1
	if f.Actor != "" {
		n++
		q += fmt.Sprintf(" AND a.actor=$%d", n)
		args = append(args, f.Actor)
	}
	if f.InitiativeID != "" {
		n++
		q += fmt.Sprintf(" AND a.issue_id IN (SELECT i.id FROM issues i JOIN projects p ON p.id=i.project_id WHERE p.initiative_id=$%d)", n)
		args = append(args, f.InitiativeID)
	}
	if f.ProjectID != "" {
		n++
		q += fmt.Sprintf(" AND a.issue_id IN (SELECT id FROM issues WHERE project_id=$%d)", n)
		args = append(args, f.ProjectID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	n++
	q += fmt.Sprintf(" ORDER BY a.created_at DESC LIMIT $%d", n)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return collectActivity(rows)
}
