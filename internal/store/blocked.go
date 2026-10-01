package store

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/johnreginald/donewhen/internal/models"
)

// MaxBlockedReason is the longest reason a move to Blocked may carry.
const MaxBlockedReason = 500

// ErrReasonRequired refuses a move to Blocked that does not say why. It wraps
// ErrInvalid, so a caller that only knows ErrInvalid still gets the message.
var ErrReasonRequired = invalid("a reason (1-%d characters) is required when moving to Blocked", MaxBlockedReason)

// CheckBlockedReason trims a reason and says whether it is acceptable.
func CheckBlockedReason(raw string) (string, error) {
	r := strings.TrimSpace(raw)
	if r == "" || utf8.RuneCountInString(r) > MaxBlockedReason {
		return "", ErrReasonRequired
	}
	return r, nil
}

// blockedReasonTx runs inside UpdateIssue when the issue changes state. It does
// nothing unless the target state is Blocked; then it requires a reason and
// stores it as a blocked_reason comment in the same transaction as the move.
func (s *Store) blockedReasonTx(ctx context.Context, tx pgx.Tx, issueID, stateID, reason, actor string) error {
	var name string
	if err := tx.QueryRow(ctx, `SELECT name FROM workflow_states WHERE id=$1`, stateID).Scan(&name); err != nil {
		return err
	}
	if !strings.EqualFold(name, "Blocked") {
		return nil
	}
	r, err := CheckBlockedReason(reason)
	if err != nil {
		return err
	}
	if actor == "" {
		actor = "human"
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO comments (issue_id, body_md, actor, kind) VALUES ($1,$2,$3,'blocked_reason')`,
		issueID, r, actor)
	return err
}

// ListBlocked returns every Blocked issue in the workspace, oldest blocked
// first, with the reason it was given and the issues it still waits on.
func (s *Store) ListBlocked(ctx context.Context, wsID string) ([]models.BlockedItem, error) {
	const lastMove = `SELECT %s FROM activity a
		WHERE a.issue_id = i.id AND a.kind = 'state_changed' AND a.to_val = 'Blocked'
		ORDER BY a.created_at DESC LIMIT 1`
	rows, err := s.pool.Query(ctx, `
		SELECT `+issueCols+`,
			coalesce((`+strings.Replace(lastMove, "%s", "a.created_at", 1)+`), i.updated_at) AS since,
			coalesce((`+strings.Replace(lastMove, "%s", "a.actor", 1)+`), 'human') AS actor,
			coalesce((SELECT c.body_md FROM comments c
				WHERE c.issue_id = i.id AND c.kind = 'blocked_reason'
				ORDER BY c.created_at DESC LIMIT 1), '') AS reason,
			ARRAY(SELECT b.key FROM issue_blockers ib
				JOIN issues b ON b.id = ib.blocker_id
				JOIN workflow_states bs ON bs.id = b.state_id
				WHERE ib.issue_id = i.id AND bs.category NOT IN ('completed', 'canceled')
				ORDER BY b.number) AS waiting_on
		FROM issues i
		WHERE i.workspace_id = $1
		  AND i.state_id IN (SELECT id FROM workflow_states WHERE workspace_id = $1 AND lower(name) = 'blocked')
		ORDER BY since ASC`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.BlockedItem
	var issues []models.Issue
	for rows.Next() {
		var is models.Issue
		var since time.Time
		var actor, reason string
		var waiting []string
		if err := rows.Scan(append(issueDest(&is), &since, &actor, &reason, &waiting)...); err != nil {
			return nil, err
		}
		issues = append(issues, is)
		items = append(items, models.BlockedItem{Issue: is, Reason: reason, Since: since, Actor: actor, WaitingOn: waiting})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := s.attachLabels(ctx, issues); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Issue = issues[i]
		if items[i].WaitingOn == nil {
			items[i].WaitingOn = []string{}
		}
	}
	return items, nil
}
