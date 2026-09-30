package store

import (
	"context"
	"fmt"
	"strings"

	"raenil/internal/models"
)

// Blocker is a ticket another waits on ("blocked by").
type Blocker struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	State    string `json:"state"`
	Category string `json:"category"`
	// Done: no longer in the way — Done, Canceled, or In Review. A
	// dependent of an In Review blocker starts from the blocker's branch, so
	// it builds on that work before it is merged.
	Done bool `json:"done"`
}

// BlockLink is one "blocked by" edge in a workspace, for views that show
// every ticket at once.
type BlockLink struct {
	IssueID   string `json:"issueId"`
	BlockerID string `json:"blockerId"`
	Done      bool   `json:"done"`
}

const blockerSelect = `
	SELECT b.id, b.key, b.title, st.name, st.category, (st.category IN ('completed', 'canceled') OR st.name = 'In Review')
	FROM issue_blockers ib
	JOIN issues b ON b.id = ib.%s
	JOIN workflow_states st ON st.id = b.state_id
	WHERE ib.%s = $1 AND b.workspace_id = $2
	ORDER BY b.number`

func (s *Store) listLinked(ctx context.Context, wsID, issueID, other, self string) ([]Blocker, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(blockerSelect, other, self), issueID, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Blocker{}
	for rows.Next() {
		var b Blocker
		if err := rows.Scan(&b.ID, &b.Key, &b.Title, &b.State, &b.Category, &b.Done); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListBlockers lists the tickets that block an issue.
func (s *Store) ListBlockers(ctx context.Context, wsID, issueID string) ([]Blocker, error) {
	return s.listLinked(ctx, wsID, issueID, "blocker_id", "issue_id")
}

// ListBlocking lists the tickets an issue blocks.
func (s *Store) ListBlocking(ctx context.Context, wsID, issueID string) ([]Blocker, error) {
	return s.listLinked(ctx, wsID, issueID, "issue_id", "blocker_id")
}

// ListBlockLinks lists every "blocked by" edge in a workspace.
func (s *Store) ListBlockLinks(ctx context.Context, wsID string) ([]BlockLink, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ib.issue_id, ib.blocker_id, (st.category IN ('completed', 'canceled') OR st.name = 'In Review')
		FROM issue_blockers ib
		JOIN issues b ON b.id = ib.blocker_id
		JOIN workflow_states st ON st.id = b.state_id
		WHERE b.workspace_id = $1`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BlockLink{}
	for rows.Next() {
		var l BlockLink
		if err := rows.Scan(&l.IssueID, &l.BlockerID, &l.Done); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetBlockers replaces the tickets that block an issue. Each must be in the
// same workspace, not the issue itself, and must not already depend on it —
// a cycle would block both forever.
func (s *Store) SetBlockers(ctx context.Context, wsID, issueID string, refs []string) ([]Blocker, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		b, err := s.issueByRef(ctx, wsID, ref)
		if err != nil {
			return nil, fmt.Errorf("%w: no ticket %q in this workspace", ErrInvalid, ref)
		}
		if b.ID == issueID {
			return nil, invalid("a ticket cannot block itself")
		}
		if cyc, err := s.dependsOn(ctx, b.ID, issueID); err != nil {
			return nil, err
		} else if cyc {
			return nil, invalid("%s already waits on this ticket; making it a blocker would block both forever", b.Key)
		}
		ids = append(ids, b.ID)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM issue_blockers WHERE issue_id = $1`, issueID); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `INSERT INTO issue_blockers (issue_id, blocker_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			issueID, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.ListBlockers(ctx, wsID, issueID)
}

// dependsOn reports whether from waits on to, directly or through others.
func (s *Store) dependsOn(ctx context.Context, from, to string) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, `
		WITH RECURSIVE up(id) AS (
			SELECT blocker_id FROM issue_blockers WHERE issue_id = $1
			UNION
			SELECT ib.blocker_id FROM issue_blockers ib JOIN up ON ib.issue_id = up.id
		)
		SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, from, to).Scan(&found)
	return found, err
}
func (s *Store) issueByRef(ctx context.Context, wsID, ref string) (models.Issue, error) {
	if len(ref) == 36 && ref[8] == '-' && ref[13] == '-' && ref[18] == '-' && ref[23] == '-' {
		return s.GetIssue(ctx, wsID, ref)
	}
	return s.GetIssueByKey(ctx, wsID, ref)
}
