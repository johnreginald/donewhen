package store

import (
	"context"
	"strings"
	"time"
)

// Evidence is one file shown on a ticket's review: a screenshot or a link.
type Evidence struct {
	ID          string    `json:"id"`
	IssueID     string    `json:"issueId"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Size        int       `json:"size"`
	Text        string    `json:"text,omitempty"` // for text/uri-list and text/plain
	CreatedAt   time.Time `json:"createdAt"`
}

// MaxEvidenceBytes caps one evidence file.
const MaxEvidenceBytes = 8 << 20

// ReplaceEvidence swaps a ticket's evidence for a new set: evidence describes
// the change as it stands, not every version of it.
func (s *Store) ReplaceEvidence(ctx context.Context, wsID, issueID string, files map[string][]byte, types map[string]string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM evidence WHERE workspace_id = $1 AND issue_id::text = $2`, wsID, issueID); err != nil {
		return 0, err
	}
	n := 0
	for name, data := range files {
		if len(data) > MaxEvidenceBytes {
			return 0, invalid("%s is larger than %d MB", name, MaxEvidenceBytes>>20)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO evidence (workspace_id, issue_id, name, content_type, data) VALUES ($1, $2, $3, $4, $5)`,
			wsID, issueID, name, types[name], data); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit(ctx)
}

// ListEvidence is a ticket's evidence, without the bytes of images.
func (s *Store) ListEvidence(ctx context.Context, wsID, issueID string) ([]Evidence, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, issue_id, name, content_type, length(data),
		       CASE WHEN content_type LIKE 'text/%' THEN convert_from(data, 'UTF8') ELSE '' END, created_at
		  FROM evidence WHERE workspace_id = $1 AND issue_id::text = $2 ORDER BY name`, wsID, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Evidence
	for rows.Next() {
		var e Evidence
		if err := rows.Scan(&e.ID, &e.IssueID, &e.Name, &e.ContentType, &e.Size, &e.Text, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Text = strings.TrimSpace(e.Text)
		out = append(out, e)
	}
	return out, rows.Err()
}

// EvidenceFile is one file's bytes.
func (s *Store) EvidenceFile(ctx context.Context, wsID, id string) (string, []byte, error) {
	var ct string
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT content_type, data FROM evidence WHERE workspace_id = $1 AND id::text = $2`, wsID, id).Scan(&ct, &data)
	return ct, data, err
}
