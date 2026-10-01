package store

import (
	"context"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/johnreginald/donewhen/internal/models"
)

// The timeline is written inside the transaction of the change it describes:
// the change and its row commit together or not at all. Anything that cannot be
// written (the insert fails) rolls the change back and the caller gets the error.

// execer is what a pool and a transaction have in common for writes.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// insertActivity appends one timeline row through db (a pool or a transaction).
func insertActivity(ctx context.Context, db execer, wsID string, a models.Activity) error {
	actor := a.Actor
	if actor == "" {
		actor = "human"
	}
	_, err := db.Exec(ctx, `
		INSERT INTO activity (workspace_id, issue_id, issue_key, issue_title, actor, kind, field, from_val, to_val, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		wsID, a.IssueID, a.IssueKey, a.IssueTitle, actor, a.Kind, a.Field, a.FromVal, a.ToVal, a.Detail)
	return err
}

// PriorityLabel is the name the timeline shows for a priority number.
func PriorityLabel(p int) string {
	switch p {
	case 1:
		return "Urgent"
	case 2:
		return "High"
	case 3:
		return "Medium"
	case 4:
		return "Low"
	}
	return "No priority"
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// labelNames is the sorted, comma-joined label names of an issue.
func labelNames(ls []models.Label) string {
	names := make([]string, len(ls))
	for i, l := range ls {
		names[i] = l.Name
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// scalarTx reads one text value, "" when the row is missing.
func scalarTx(ctx context.Context, tx pgx.Tx, query string, arg any) (string, error) {
	var v string
	err := tx.QueryRow(ctx, query, arg).Scan(&v)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return v, err
}

// issueChanges is the timeline rows for an issue going from before to after:
// one per field that actually changed, none for a no-op.
func (s *Store) issueChanges(ctx context.Context, tx pgx.Tx, before, after models.Issue, actor, blockedReason string) ([]models.Activity, error) {
	base := models.Activity{IssueID: &after.ID, IssueKey: after.Key, IssueTitle: after.Title, Actor: actor}
	var out []models.Activity
	add := func(kind, field, from, to string) {
		e := base
		e.Kind, e.Field, e.FromVal, e.ToVal = kind, field, from, to
		out = append(out, e)
	}
	if before.StateID != after.StateID {
		from, err := scalarTx(ctx, tx, `SELECT name FROM workflow_states WHERE id=$1`, before.StateID)
		if err != nil {
			return nil, err
		}
		to, err := scalarTx(ctx, tx, `SELECT name FROM workflow_states WHERE id=$1`, after.StateID)
		if err != nil {
			return nil, err
		}
		add("state_changed", "status", from, to)
		if strings.EqualFold(to, "Blocked") {
			out[len(out)-1].Detail = strings.TrimSpace(blockedReason) // why, on the timeline row too
		}
	}
	if before.Priority != after.Priority {
		add("priority_changed", "priority", PriorityLabel(before.Priority), PriorityLabel(after.Priority))
	}
	if ptrStr(before.ProjectID) != ptrStr(after.ProjectID) {
		name := func(id *string) (string, error) {
			if id == nil || *id == "" {
				return "None", nil
			}
			return scalarTx(ctx, tx, `SELECT name FROM projects WHERE id=$1`, *id)
		}
		from, err := name(before.ProjectID)
		if err != nil {
			return nil, err
		}
		to, err := name(after.ProjectID)
		if err != nil {
			return nil, err
		}
		add("epic_changed", "epic", from, to)
	}
	if before.Title != after.Title {
		add("title_changed", "title", before.Title, after.Title)
	}
	if bl, al := labelNames(before.Labels), labelNames(after.Labels); bl != al {
		add("labels_changed", "labels", bl, al)
	}
	if ptrStr(before.AssigneeID) != ptrStr(after.AssigneeID) {
		email := func(id *string) (string, error) {
			if id == nil || *id == "" {
				return "", nil
			}
			return scalarTx(ctx, tx, `SELECT email FROM users WHERE id=$1`, *id)
		}
		from, err := email(before.AssigneeID)
		if err != nil {
			return nil, err
		}
		to, err := email(after.AssigneeID)
		if err != nil {
			return nil, err
		}
		add("assignee_changed", "assignee", from, to)
	}
	if before.DescriptionMD != after.DescriptionMD {
		add("description_changed", "description", "", "") // no body copy
	}
	return out, nil
}

// gateOverrideRow is the timeline row for a forced move past the done-when gate.
func gateOverrideRow(is models.Issue, stateName, actor string, g *GateOutcome) (models.Activity, bool) {
	if g == nil || !g.Overridden || g.Err == nil {
		return models.Activity{}, false
	}
	return models.Activity{
		IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
		Kind: "gate_overridden", Field: "status", ToVal: stateName, Detail: g.Err.Error(),
	}, true
}
