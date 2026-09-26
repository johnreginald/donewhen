package store

import (
	"context"
	"fmt"
	"strings"

	"raenil/internal/models"
)

// Blocker is a ticket that must be Done before another may run.
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

// OpenBlockers are the blockers still in the way: not Done, Canceled or In
// Review.
func (s *Store) OpenBlockers(ctx context.Context, wsID, issueID string) ([]Blocker, error) {
	all, err := s.ListBlockers(ctx, wsID, issueID)
	if err != nil {
		return nil, err
	}
	var open []Blocker
	for _, b := range all {
		if !b.Done {
			open = append(open, b)
		}
	}
	return open, nil
}

// BlockedMessage says what stands in the way, by key and state.
func BlockedMessage(open []Blocker) string {
	parts := make([]string, len(open))
	for i, b := range open {
		parts[i] = fmt.Sprintf("%s (%s)", b.Key, b.State)
	}
	return "Blocked by " + strings.Join(parts, ", ") + " — it can run once they are In Review or Done"
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

// SetEpicAutorun turns an epic's running on or off.
func (s *Store) SetEpicAutorun(ctx context.Context, wsID, projectID string, on bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE projects SET autorun = $3 WHERE id = $1 AND workspace_id = $2`, projectID, wsID, on)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReadyToRun lists an epic's tickets in the Ready state that nothing open
// blocks and that no job is already working, most urgent first.
func (s *Store) ReadyToRun(ctx context.Context, wsID, projectID string) (ready []models.Issue, blocked []string, err error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id,
		       EXISTS (SELECT 1 FROM issue_blockers ib JOIN issues b ON b.id = ib.blocker_id
		               JOIN workflow_states bs ON bs.id = b.state_id
		               WHERE ib.issue_id = i.id AND bs.category NOT IN ('completed', 'canceled') AND bs.name <> 'In Review')
		FROM issues i JOIN workflow_states st ON st.id = i.state_id
		WHERE i.workspace_id = $1 AND i.project_id = $2 AND st.name = 'Ready'
		  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.issue_id = i.id AND j.status IN ('queued', 'claimed'))
		ORDER BY CASE WHEN i.priority = 0 THEN 5 ELSE i.priority END, i.position, i.number`, wsID, projectID)
	if err != nil {
		return nil, nil, err
	}
	type row struct {
		id      string
		blocked bool
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.blocked); err != nil {
			rows.Close()
			return nil, nil, err
		}
		rs = append(rs, r)
	}
	rows.Close()
	for _, r := range rs {
		is, err := s.GetIssue(ctx, wsID, r.id)
		if err != nil {
			return nil, nil, err
		}
		if r.blocked {
			blocked = append(blocked, is.Key)
		} else {
			ready = append(ready, is)
		}
	}
	return ready, blocked, nil
}

// AutorunDependents lists the tickets a now-Done ticket was blocking that
// sit in running epics, with the epic each belongs to.
func (s *Store) AutorunDependents(ctx context.Context, wsID, blockerID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT p.id FROM issue_blockers ib
		JOIN issues i ON i.id = ib.issue_id
		JOIN projects p ON p.id = i.project_id
		WHERE ib.blocker_id = $1 AND i.workspace_id = $2 AND p.autorun`, blockerID, wsID)
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

// issueByRef finds a ticket by its key (ACM-288) or id.
func (s *Store) issueByRef(ctx context.Context, wsID, ref string) (models.Issue, error) {
	if len(ref) == 36 && ref[8] == '-' && ref[13] == '-' && ref[18] == '-' && ref[23] == '-' {
		return s.GetIssue(ctx, wsID, ref)
	}
	return s.GetIssueByKey(ctx, wsID, ref)
}

// AddBlocker links one more ticket that must clear before an issue runs, and
// marks the issue to start again on its own once its blockers clear — the
// path an agent takes when it finds a dependency the plan missed.
func (s *Store) AddBlocker(ctx context.Context, wsID, issueID, blockerRef string) (Blocker, error) {
	current, err := s.ListBlockers(ctx, wsID, issueID)
	if err != nil {
		return Blocker{}, err
	}
	refs := []string{blockerRef}
	for _, b := range current {
		refs = append(refs, b.Key)
	}
	all, err := s.SetBlockers(ctx, wsID, issueID, refs)
	if err != nil {
		return Blocker{}, err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE issues SET run_when_unblocked = true WHERE id = $1 AND workspace_id = $2`,
		issueID, wsID); err != nil {
		return Blocker{}, err
	}
	b, _ := s.issueByRef(ctx, wsID, blockerRef)
	for _, x := range all {
		if x.ID == b.ID {
			return x, nil
		}
	}
	return Blocker{}, ErrNotFound
}

// UnblockedWaiting lists the tickets a now-cleared ticket was holding back
// that asked to start again on their own and have nothing else in the way.
func (s *Store) UnblockedWaiting(ctx context.Context, wsID, blockerID string) ([]models.Issue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id FROM issue_blockers ib
		JOIN issues i ON i.id = ib.issue_id
		JOIN workflow_states st ON st.id = i.state_id
		WHERE ib.blocker_id = $1 AND i.workspace_id = $2 AND i.run_when_unblocked
		  AND st.name = 'Ready' 
		  AND NOT EXISTS (SELECT 1 FROM issue_blockers o JOIN issues b ON b.id = o.blocker_id
		                  JOIN workflow_states bs ON bs.id = b.state_id
		                  WHERE o.issue_id = i.id AND bs.category NOT IN ('completed', 'canceled') AND bs.name <> 'In Review')
		  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.issue_id = i.id AND j.status IN ('queued', 'claimed'))`, blockerID, wsID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []models.Issue
	for _, id := range ids {
		is, err := s.GetIssue(ctx, wsID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, is)
	}
	return out, nil
}

// ClearRunWhenUnblocked drops the flag once the ticket has been started.
func (s *Store) ClearRunWhenUnblocked(ctx context.Context, wsID, issueID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE issues SET run_when_unblocked = false WHERE id = $1 AND workspace_id = $2`, issueID, wsID)
	return err
}
