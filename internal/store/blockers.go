package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Blocker is a ticket another waits on ("blocked by").
type Blocker struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	State    string `json:"state"`
	Category string `json:"category"`
	// Done: no longer in the way — Done or Canceled.
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
	SELECT b.id, b.key, b.title, st.name, st.category, st.category IN ('completed', 'canceled')
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
		SELECT ib.issue_id, ib.blocker_id, st.category IN ('completed', 'canceled')
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
//
// Everything runs in one transaction. A workspace-wide advisory lock
// serialises blocker writes so two concurrent requests (A→B and B→A) cannot
// each pass the cycle check against the other's uncommitted edge; the issue
// row is locked as well.
func (s *Store) SetBlockers(ctx context.Context, wsID, issueID string, refs []string) ([]Blocker, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('issue_blockers:' || $1::text))`, wsID); err != nil {
		return nil, err
	}
	var locked string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM issues WHERE id::text = $1 AND workspace_id = $2 FOR UPDATE`, issueID, wsID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	issueID = locked

	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		var bid, bkey string
		err := tx.QueryRow(ctx,
			`SELECT id, key FROM issues WHERE workspace_id = $1 AND (id::text = $2 OR upper(key) = upper($2))`,
			wsID, ref).Scan(&bid, &bkey)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalid("invalid_blocker: no ticket %q in this workspace", ref)
		}
		if err != nil {
			return nil, err
		}
		if bid == issueID {
			return nil, invalid("a ticket cannot block itself")
		}
		if cyc, err := dependsOn(ctx, tx, bid, issueID); err != nil {
			return nil, err
		} else if cyc {
			return nil, invalid("%s already waits on this ticket; making it a blocker would block both forever", bkey)
		}
		ids = append(ids, bid)
	}
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
func dependsOn(ctx context.Context, tx pgx.Tx, from, to string) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE up(id) AS (
			SELECT blocker_id FROM issue_blockers WHERE issue_id = $1
			UNION
			SELECT ib.blocker_id FROM issue_blockers ib JOIN up ON ib.issue_id = up.id
		)
		SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, from, to).Scan(&found)
	return found, err
}
