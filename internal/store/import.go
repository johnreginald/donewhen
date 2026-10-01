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
	Team        string `json:"team"` // Linear team → DoneWhen initiative
}

type ImportIssue struct {
	ID          string `json:"id"` // human key, e.g. "ACM-155" — preserved verbatim
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    struct {
		Value int `json:"value"`
	} `json:"priority"`
	Status     string    `json:"status"`     // workflow state name
	StatusType string    `json:"statusType"` // linear category, for fallback mapping
	Labels     []string  `json:"labels"`
	Project    string    `json:"project"` // project name ("" = none)
	Team       string    `json:"team"`    // Linear team → initiative (groups the project)
	ParentID   string    `json:"parentId"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// ImportResult summarizes what an import did.
type ImportResult struct {
	Initiatives    int      `json:"initiatives"`    // initiatives created (from teams)
	Projects       int      `json:"projects"`       // projects created
	Issues         int      `json:"issues"`         // issues created
	Labels         int      `json:"labels"`         // labels created
	Skipped        int      `json:"skipped"`        // issues skipped (key already existed)
	SkippedKeys    []string `json:"skippedKeys"`    // which keys were skipped
	UnmappedStates []string `json:"unmappedStates"` // statuses that fell back to the default state
}

// UpdateDescriptions overwrites description_md for the given issue keys (used to
// backfill full bodies after an import that carried truncated descriptions).
// One transaction, no events. Returns how many rows were updated.
func (s *Store) UpdateDescriptions(ctx context.Context, wsID string, byKey map[string]string) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	n := 0
	for key, desc := range byKey {
		ct, err := tx.Exec(ctx,
			`UPDATE issues SET description_md=$1, updated_at=now() WHERE key=$2 AND workspace_id=$3`, desc, key, wsID)
		if err != nil {
			return n, err
		}
		n += int(ct.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return n, err
	}
	return n, nil
}

// linearTypeToState maps a Linear workflow-state category to a DoneWhen state name,
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
func (s *Store) Import(ctx context.Context, wsID string, data ImportData) (ImportResult, error) {
	var res ImportResult
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)

	// --- states: name -> id, plus a default for unmatched statuses ---
	stateByName := map[string]string{} // lower(name) -> id
	rows, err := tx.Query(ctx, `SELECT id, name FROM workflow_states WHERE workspace_id=$1`, wsID)
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
		if err := tx.QueryRow(ctx,
			`SELECT id FROM workflow_states WHERE workspace_id=$1 ORDER BY position LIMIT 1`, wsID).Scan(&defaultState); err != nil {
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

	// --- initiatives from Linear teams: ensure by name, cache name -> id ---
	iniByName := map[string]string{}
	ensureInitiative := func(name string) (string, error) {
		if name == "" {
			return "", nil
		}
		if id, ok := iniByName[name]; ok {
			return id, nil
		}
		var id string
		err := tx.QueryRow(ctx,
			`SELECT id FROM initiatives WHERE name=$1 AND workspace_id=$2 LIMIT 1`, name, wsID).Scan(&id)
		if err == pgx.ErrNoRows {
			if err := tx.QueryRow(ctx,
				`INSERT INTO initiatives (workspace_id, name) VALUES ($1,$2) RETURNING id`, wsID, name).Scan(&id); err != nil {
				return "", err
			}
			res.Initiatives++
		} else if err != nil {
			return "", err
		}
		iniByName[name] = id
		return id, nil
	}

	// --- projects: ensure by name, grouped under their team's initiative ---
	projByName := map[string]string{}
	ensureProject := func(name, desc, team string) (string, error) {
		if name == "" {
			return "", nil
		}
		var iniID *string
		if team != "" {
			iid, err := ensureInitiative(team)
			if err != nil {
				return "", err
			}
			if iid != "" {
				iniID = &iid
			}
		}
		// Link an existing project to its initiative if it has none yet
		// (idempotent enrichment for a re-run).
		link := func(id string) error {
			if iniID == nil {
				return nil
			}
			_, err := tx.Exec(ctx,
				`UPDATE projects SET initiative_id=$1 WHERE id=$2 AND initiative_id IS NULL AND workspace_id=$3`, *iniID, id, wsID)
			return err
		}
		if id, ok := projByName[name]; ok {
			return id, link(id)
		}
		var id string
		err := tx.QueryRow(ctx,
			`SELECT id FROM projects WHERE name=$1 AND workspace_id=$2 LIMIT 1`, name, wsID).Scan(&id)
		if err == nil {
			projByName[name] = id
			return id, link(id)
		}
		if err != pgx.ErrNoRows {
			return "", err
		}
		if err := tx.QueryRow(ctx,
			`INSERT INTO projects (workspace_id, name, description_md, initiative_id) VALUES ($1,$2,$3,$4) RETURNING id`,
			wsID, name, desc, iniID).Scan(&id); err != nil {
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
		if _, err := ensureProject(p.Name, desc, p.Team); err != nil {
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
			`INSERT INTO labels (workspace_id, name) VALUES ($1,$2)
			 ON CONFLICT (workspace_id, name) DO UPDATE SET name=EXCLUDED.name
			 RETURNING id, (xmax = 0)`, wsID, name).Scan(&id, &inserted)
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
			pid, err := ensureProject(is.Project, "", is.Team)
			if err != nil {
				return res, err
			}
			projectID = &pid
		}

		// Imported issues keep their own key, so only the per-workspace number
		// counter advances here; issue_seq is reconciled once, after the loop.
		var number int64
		if err := tx.QueryRow(ctx,
			`UPDATE workspaces SET number_seq = number_seq + 1 WHERE id=$1 RETURNING number_seq`, wsID).Scan(&number); err != nil {
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
			  (workspace_id, number, key, title, description_md, state_id, project_id, priority, position, parent_key, created_at, updated_at)
			VALUES ($12,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
			number, is.ID, is.Title, is.Description,
			resolveState(is.Status, is.StatusType), projectID, is.Priority.Value,
			float64(number), parentKey, created, updated, wsID,
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

	// A parent key is free text (the parent may sit later in the file), but it
	// must never name an issue that lives in another workspace.
	for _, is := range data.Issues {
		if is.ParentID == "" {
			continue
		}
		var foreign bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM issues WHERE upper(key)=upper($1) AND workspace_id <> $2)
			   AND NOT EXISTS(SELECT 1 FROM issues WHERE upper(key)=upper($1) AND workspace_id = $2)`,
			is.ParentID, wsID).Scan(&foreign); err != nil {
			return res, err
		}
		if foreign {
			return res, invalid("invalid_parent: %s names an issue in another workspace", is.ParentID)
		}
	}

	// An import can land keys that share this workspace's prefix (e.g. importing
	// ACM-300 into the ACM workspace). Lift issue_seq past them so the next
	// natively-created issue does not propose a key that already exists.
	if _, err := tx.Exec(ctx, `
		UPDATE workspaces w SET issue_seq = GREATEST(w.issue_seq, coalesce((
			SELECT max(coalesce(nullif(regexp_replace(split_part(i.key,'-',2), '[^0-9]', '', 'g'), ''), '0')::bigint)
			  FROM issues i WHERE upper(split_part(i.key,'-',1)) = w.key_prefix), 0))
		WHERE w.id = $1`, wsID); err != nil {
		return res, err
	}

	for st := range unmapped {
		res.UnmappedStates = append(res.UnmappedStates, st)
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}
