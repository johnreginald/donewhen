package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Rank says where a card lands in a column: below After, above Before. Either
// may be empty. The client sends the neighbours it sees at the drop point, so
// cards hidden by a filter keep their order.
type Rank struct {
	After, Before string
}

// minRankGap is the smallest distance two neighbours may have before the
// column is renumbered, so repeated halving never runs out of float precision.
const minRankGap = 1e-6

// rankTx returns the position that puts issue id between r.After and r.Before
// in column stateID (empty = the issue's current column). If the neighbours
// are too close it renumbers the column in the same transaction.
func (s *Store) rankTx(ctx context.Context, tx pgx.Tx, wsID, id, stateID string, r Rank) (float64, error) {
	if stateID == "" {
		if err := tx.QueryRow(ctx,
			`SELECT state_id FROM issues WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, id, wsID).Scan(&stateID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, ErrNotFound
			}
			return 0, err
		}
	}
	if r.After != "" && r.After == r.Before || r.After == id || r.Before == id {
		return 0, invalid("a card cannot be placed relative to itself")
	}

	// The column, without the moved card, in board order.
	type card struct {
		id  string
		pos float64
	}
	rows, err := tx.Query(ctx,
		`SELECT id, position FROM issues
		 WHERE workspace_id=$1 AND state_id=$2 AND id<>$3
		 ORDER BY position, number`, wsID, stateID, id)
	if err != nil {
		return 0, err
	}
	var col []card
	for rows.Next() {
		var c card
		if err := rows.Scan(&c.id, &c.pos); err != nil {
			rows.Close()
			return 0, err
		}
		col = append(col, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	at := func(ref string) (int, error) {
		if ref == "" {
			return -1, nil
		}
		for i, c := range col {
			if c.id == ref {
				return i, nil
			}
		}
		return -1, invalid("neighbour %q is not in the target column", ref)
	}
	ai, err := at(r.After)
	if err != nil {
		return 0, err
	}
	bi, err := at(r.Before)
	if err != nil {
		return 0, err
	}

	// slot = index in col the moved card is inserted at.
	var slot int
	switch {
	case r.After != "":
		slot = ai + 1
	case r.Before != "":
		slot = bi
	default:
		slot = len(col) // no neighbours: the end of the column
	}
	if r.After != "" && r.Before != "" && bi != ai+1 {
		// The client's view is stale: the two cards are no longer adjacent.
		// Trust "after" and drop in right below it.
		slot = ai + 1
	}

	var lo, hi *float64
	if slot > 0 {
		v := col[slot-1].pos
		lo = &v
	}
	if slot < len(col) {
		v := col[slot].pos
		hi = &v
	}
	switch {
	case lo != nil && hi != nil:
		if *hi-*lo >= minRankGap {
			return (*lo + *hi) / 2, nil
		}
	case lo != nil:
		return *lo + 1, nil
	case hi != nil:
		return *hi - 1, nil
	default:
		return 0, nil
	}

	// Gap too small: renumber the column 1..n around the moved card.
	upIDs := make([]string, 0, len(col))
	upPos := make([]float64, 0, len(col))
	var moved float64
	n := 0
	for i, c := range col {
		if i == slot {
			n++
			moved = float64(n)
		}
		n++
		upIDs, upPos = append(upIDs, c.id), append(upPos, float64(n))
	}
	if slot == len(col) {
		moved = float64(n + 1)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE issues SET position = v.pos
		 FROM unnest($1::uuid[], $2::float8[]) AS v(id, pos)
		 WHERE issues.id = v.id AND issues.workspace_id = $3`, upIDs, upPos, wsID); err != nil {
		return 0, err
	}
	return moved, nil
}
