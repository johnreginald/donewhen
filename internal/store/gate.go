package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// The done-when gate. An issue may enter a gated state only when its
// checklist is complete. The check runs inside the same transaction as the
// state change, so a criterion unticked a moment earlier cannot slip past it.

// Stable machine-readable codes carried by GateError.
const (
	GateCriteriaIncomplete = "criteria_incomplete"
	GateCriteriaMissing    = "criteria_missing"
)

// gatedStates are the states that claim the work is finished or ready to be
// judged. Compared case-insensitively, as state names are everywhere else.
var gatedStates = map[string]bool{"in review": true, "done": true}

// IsGatedState reports whether moving into the named state needs a complete
// done-when checklist.
func IsGatedState(name string) bool { return gatedStates[strings.ToLower(name)] }

// OpenCriterion is one unticked item, numbered as get_criteria numbers it.
type OpenCriterion struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Kind  string `json:"kind"`
}

// GateError refuses a move into a gated state.
type GateError struct {
	Code  string
	State string
	Open  []OpenCriterion
}

func (e *GateError) Error() string {
	if e.Code == GateCriteriaMissing {
		return fmt.Sprintf("%s: cannot move to %s: the issue has no done-when criteria; add them with set_criteria first",
			e.Code, e.State)
	}
	parts := make([]string, len(e.Open))
	for i, o := range e.Open {
		parts[i] = fmt.Sprintf("%d. %s", o.Index, o.Text)
	}
	noun := "criteria"
	if len(e.Open) == 1 {
		noun = "criterion"
	}
	return fmt.Sprintf("%s: cannot move to %s: %d done-when %s not done: %s",
		e.Code, e.State, len(e.Open), noun, strings.Join(parts, "; "))
}

// GateOutcome reports, to the caller that asked for the move, that the gate
// was overridden rather than satisfied. Filled by UpdateIssue/CreateIssue.
type GateOutcome struct {
	Overridden bool
	Err        *GateError // what the override waved through
}

// evalGate decides a move. Open items of kind judgment are advisory (see
// models.CriterionJudgment) and never block; a checklist with no criteria at
// all is refused.
func evalGate(crit []models.Criterion, stateName string) *GateError {
	if len(crit) == 0 {
		return &GateError{Code: GateCriteriaMissing, State: stateName}
	}
	var open []OpenCriterion
	for i, c := range crit {
		if !c.Done && c.Kind != models.CriterionJudgment {
			open = append(open, OpenCriterion{Index: i + 1, Text: c.Body, Kind: c.Kind})
		}
	}
	if len(open) == 0 {
		return nil
	}
	return &GateError{Code: GateCriteriaIncomplete, State: stateName, Open: open}
}

// gateTx applies the gate for a move of issueID ("" for a new issue) into
// stateID. It returns a *GateError when the move is refused. With force, a
// refusal is recorded in out and the move allowed.
func (s *Store) gateTx(ctx context.Context, tx pgx.Tx, issueID, stateID string, force bool, out *GateOutcome) error {
	var name string
	if err := tx.QueryRow(ctx, `SELECT name FROM workflow_states WHERE id=$1`, stateID).Scan(&name); err != nil {
		return err
	}
	if !IsGatedState(name) {
		return nil
	}
	var crit []models.Criterion
	if issueID != "" {
		// FOR SHARE: a concurrent untick must wait for this move to commit.
		rows, err := tx.Query(ctx, `
			SELECT `+criterionCols+` FROM issue_criteria c
			WHERE c.issue_id=$1 ORDER BY c.position, c.created_at FOR SHARE OF c`, issueID)
		if err != nil {
			return err
		}
		for rows.Next() {
			c, err := scanCriterion(rows)
			if err != nil {
				rows.Close()
				return err
			}
			crit = append(crit, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	ge := evalGate(crit, name)
	if ge == nil {
		return nil
	}
	if force {
		if out != nil {
			out.Overridden, out.Err = true, ge
		}
		return nil
	}
	return ge
}
