package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ImportData is a tracker export in Linear's native shape (the fields we keep).
// Unknown fields in the JSON are ignored by the decoder.
type ImportData struct {
	Projects []ImportProject `json:"projects"`
	Issues   []ImportIssue   `json:"issues"`
}

type ImportProject struct {
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

type ImportIssue struct {
	ID          string    `json:"id"` // human key, e.g. "PP-155" — preserved verbatim
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    struct {
		Value int `json:"value"`
	} `json:"priority"`
	Status     string    `json:"status"`     // workflow state name
	StatusType string    `json:"statusType"` // linear category, for fallback mapping
	Labels     []string  `json:"labels"`
	Project    string    `json:"project"` // project name ("" = none)
	ParentID   string    `json:"parentId"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ImportResult summarizes what an import did.
type ImportResult struct {
	Projects       int      `json:"projects"`       // projects created
	Issues         int      `json:"issues"`         // issues created
	Labels         int      `json:"labels"`         // labels created
	Skipped        int      `json:"skipped"`        // issues skipped (key already existed)
	SkippedKeys    []string `json:"skippedKeys"`    // which keys were skipped
	UnmappedStates []string `json:"unmappedStates"` // statuses that fell back to the default state
}

// linearTypeToState maps a Linear workflow-state category to a Raenil state name,
// used when an issue's status name has no exact match.
var linearTypeToState = map[string]string{
	"triage":    "Triage",
	"backlog":   "Backlog",
	"unstarted": "Ready",
	"started":   "In Progress",
	"completed": "Done",
	"canceled":  "Canceled",
	"duplicate": "Canceled",
}

// Import loads an external tracker export, preserving issue keys, timestamps,
// state, priority, project membership, labels, and sub-issue parent keys. It is
// idempotent: issues whose key already exists are skipped. Runs in one
// transaction and emits no events (bulk import must not spam push/SSE).
func (s *Store) Import(ctx context.Context, data ImportData) (ImportResult, error) {
	var res ImportResult
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)

	// --- states: name -> id, plus a default for unmatched statuses ---
	stateByName := map[string]string{} // lower(name) -> id
	rows, err := tx.Query(ctx, `SELECT id, name FROM workflow_states`)
	if err != nil {
		return res, err
	}
	var defaultState string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return res, err
		}
		stateByName[strings.ToLower(name)] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	if id, ok := stateByName["backlog"]; ok {
		defaultState = id
	} else {
		if err := tx.QueryRow(ctx, `SELECT id FROM workflow_states ORDER BY position LIMIT 1`).Scan(&defaultState); err != nil {
			return res, err
		}
	}
	unmapped := map[string]bool{}
	resolveState := func(status, statusType string) string {
		if id, ok := stateByName[strings.ToLower(status)]; ok {
			return id
		}
		if name, ok := linearTypeToState[strings.ToLower(statusType)]; ok {
			if id, ok := stateByName[strings.ToLower(name)]; ok {
				return id
			}
		}
		if status != "" {
			unmapped[status] = true
		}
		return defaultState
	}

	// --- projects: ensure by name, cache name -> id ---
	projByName := map[string]string{}
	ensureProject := func(name, desc string) (string, error) {
		if name == "" {
			return "", nil
		}
		if id, ok := projByName[name]; ok {
			return id, nil
		}
		var id string
		err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE name=$1 LIMIT 1`, name).Scan(&id)
		if err == nil {
			projByName[name] = id
			return id, nil
		}
		if err != pgx.ErrNoRows {
			return "", err
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO projects (name, description_md) VALUES ($1,$2) RETURNING id`,
			name, desc).Scan(&id); err != nil {
			return "", err
		}
		projByName[name] = id
		res.Projects++
		return id, nil
	}
	for _, p := range data.Projects {
		desc := p.Description
		if desc == "" {
			desc = p.Summary
		}
		if _, err := ensureProject(p.Name, desc); err != nil {
			return res, err
		}
	}

	// --- labels: ensure by exact name, cache name -> id (all attached directly,
	// bypassing exclusive-group enforcement so no label is dropped) ---
	labelByName := map[string]string{}
	ensureLabel := func(name string) (string, error) {
		if id, ok := labelByName[name]; ok {
			return id, nil
		}
		var id string
		var inserted bool // xmax = 0 on a fresh insert; nonzero when the row pre-existed
		err := tx.QueryRow(ctx,
			`INSERT INTO labels (name) VALUES ($1)
			 ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name
			 RETURNING id, (xmax = 0)`, name).Scan(&id, &inserted)
		if err != nil {
			return "", err
		}
		if inserted {
			res.Labels++
		}
		labelByName[name] = id
		return id, nil
	}

	// --- issues ---
	for _, is := range data.Issues {
		if is.ID == "" {
			continue
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT true FROM issues WHERE key=$1`, is.ID).Scan(&exists); err == nil {
			res.Skipped++
			res.SkippedKeys = append(res.SkippedKeys, is.ID)
			continue
		} else if err != pgx.ErrNoRows {
			return res, err
		}

		var projectID *string
		if is.Project != "" {
			pid, err := ensureProject(is.Project, "")
			if err != nil {
				return res, err
			}
			projectID = &pid
		}

		var number int
		if err := tx.QueryRow(ctx, `SELECT nextval('issue_number_seq')`).Scan(&number); err != nil {
			return res, err
		}

		created := is.CreatedAt
		if created.IsZero() {
			created = time.Now()
		}
		updated := is.UpdatedAt
		if updated.IsZero() {
			updated = created
		}
		var parentKey *string
		if is.ParentID != "" {
			parentKey = &is.ParentID
		}

		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO issues
			  (number, key, title, description_md, state_id, project_id, priority, position, parent_key, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
			number, is.ID, is.Title, is.Description,
			resolveState(is.Status, is.StatusType), projectID, is.Priority.Value,
			float64(number), parentKey, created, updated,
		).Scan(&id)
		if err != nil {
			return res, fmt.Errorf("insert issue %s: %w", is.ID, err)
		}

		for _, name := range is.Labels {
			if name == "" {
				continue
			}
			lid, err := ensureLabel(name)
			if err != nil {
				return res, err
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO issue_labels (issue_id, label_id) VALUES ($1,$2)
				 ON CONFLICT DO NOTHING`, id, lid); err != nil {
				return res, err
			}
		}
		res.Issues++
	}

	for st := range unmapped {
		res.UnmappedStates = append(res.UnmappedStates, st)
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}
