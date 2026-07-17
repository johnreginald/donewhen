package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// scanInboxRow scans issueCols followed by the two inbox extras
// (commit_count, entered_review_at).
func scanInboxRow(row pgx.Row) (models.Issue, int, *time.Time, error) {
	var is models.Issue
	var commitCount int
	var enteredAt *time.Time
	err := row.Scan(&is.ID, &is.Number, &is.Key, &is.Title, &is.DescriptionMD,
		&is.StateID, &is.ProjectID, &is.AssigneeID, &is.Priority, &is.Position,
		&is.DocCount, &is.ParentKey, &is.ChildCount, &is.GitBranch, &is.PRURL,
		&is.CreatedAt, &is.UpdatedAt, &commitCount, &enteredAt)
	return is, commitCount, enteredAt, err
}

// InboxNeedsReview returns issues currently in "In Review" that the AI moved
// there — the human's review queue. Newest-entered first.
func (s *Store) InboxNeedsReview(ctx context.Context) ([]models.InboxItem, error) {
	q := `
		SELECT ` + issueCols + `,
			(SELECT count(*) FROM issue_commits ic WHERE ic.issue_id = i.id) AS commit_count,
			(SELECT max(a.created_at) FROM activity a
			   WHERE a.issue_id = i.id AND a.kind = 'state_changed'
			     AND a.to_val = 'In Review' AND a.actor = 'ai') AS entered_review_at
		FROM issues i
		WHERE i.state_id = (SELECT id FROM workflow_states WHERE name = 'In Review')
		  AND EXISTS (
			SELECT 1 FROM activity a
			WHERE a.issue_id = i.id AND a.kind = 'state_changed'
			  AND a.to_val = 'In Review' AND a.actor = 'ai')
		ORDER BY entered_review_at DESC NULLS LAST`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.InboxItem
	var issues []models.Issue
	for rows.Next() {
		is, cc, entered, err := scanInboxRow(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, is)
		items = append(items, models.InboxItem{Issue: is, CommitCount: cc, EnteredReviewAt: entered})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// attachLabels mutates the issues slice in place; copy the labelled issues
	// back onto the items (which hold value copies).
	if _, err := s.attachLabels(ctx, issues); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Issue = issues[i]
	}
	return items, nil
}

// GetInboxSeen returns when the user last marked the inbox seen (nil = never).
func (s *Store) GetInboxSeen(ctx context.Context, userID string) (*time.Time, error) {
	var t *time.Time
	err := s.pool.QueryRow(ctx, `SELECT inbox_seen_at FROM users WHERE id=$1`, userID).Scan(&t)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return t, err
}

// SetInboxSeen stamps the user's inbox_seen_at to now and returns it.
func (s *Store) SetInboxSeen(ctx context.Context, userID string) (time.Time, error) {
	var t time.Time
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET inbox_seen_at=now() WHERE id=$1 RETURNING inbox_seen_at`, userID).Scan(&t)
	if err == pgx.ErrNoRows {
		return t, ErrNotFound
	}
	return t, err
}
