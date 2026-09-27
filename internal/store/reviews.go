package store

import (
	"context"
	"encoding/json"

	"raenil/internal/models"
)

const reviewCols = `id, issue_id, kind, reviewer, builder, verdict, summary, findings, guide_md, content_hash, round, created_at`

// SaveReview records an agent review on a ticket.
func (s *Store) SaveReview(ctx context.Context, wsID string, r models.Review) (models.Review, error) {
	if r.Kind != "ticket" && r.Kind != "code" {
		return r, invalid("review kind must be ticket or code, not %q", r.Kind)
	}
	if r.Verdict != "pass" && r.Verdict != "changes" {
		return r, invalid("review verdict must be pass or changes, not %q", r.Verdict)
	}
	if r.Findings == nil {
		r.Findings = []models.ReviewFinding{}
	}
	if r.Round < 1 {
		r.Round = 1
	}
	f, _ := json.Marshal(r.Findings)
	row := s.pool.QueryRow(ctx, `
		INSERT INTO reviews (workspace_id, issue_id, kind, reviewer, builder, verdict, summary, findings, guide_md, content_hash, round)
		SELECT $1, i.id, $3, $4, $5, $6, $7, $8, $9, $10, $11 FROM issues i
		 WHERE i.workspace_id = $1 AND (i.id::text = $2 OR i.key = $2)
		RETURNING `+reviewCols, wsID, r.IssueID, r.Kind, r.Reviewer, r.Builder, r.Verdict, r.Summary, f, r.GuideMD, r.ContentHash, r.Round)
	return scanReview(row)
}

// ListReviews is a ticket's reviews, newest first.
func (s *Store) ListReviews(ctx context.Context, wsID, issueID string) ([]models.Review, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+reviewCols+` FROM reviews
		WHERE workspace_id = $1 AND issue_id::text = $2 ORDER BY created_at DESC LIMIT 50`, wsID, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Review
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type rowScanner interface{ Scan(dest ...any) error }

func scanReview(row rowScanner) (models.Review, error) {
	var r models.Review
	var f []byte
	if err := row.Scan(&r.ID, &r.IssueID, &r.Kind, &r.Reviewer, &r.Builder, &r.Verdict, &r.Summary, &f,
		&r.GuideMD, &r.ContentHash, &r.Round, &r.CreatedAt); err != nil {
		return r, err
	}
	if json.Unmarshal(f, &r.Findings) != nil || r.Findings == nil {
		r.Findings = []models.ReviewFinding{}
	}
	return r, nil
}
