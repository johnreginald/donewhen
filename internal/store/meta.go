package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- Workflow states ----

func (s *Store) ListStates(ctx context.Context, wsID string) ([]models.WorkflowState, error) {
	return s.ListStatesAcross(ctx, []string{wsID})
}

// ListStatesAcross reads the board columns of several workspaces at once. Only
// the agent surface uses the Across variants: a browser is always in exactly
// one workspace, and keeping its methods single-valued preserves the
// compile-time guarantee that a request cannot accidentally widen its scope.
func (s *Store) ListStatesAcross(ctx context.Context, wsIDs []string) ([]models.WorkflowState, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, category, position, color FROM workflow_states
		 WHERE workspace_id = ANY($1) ORDER BY position`, wsIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WorkflowState
	for rows.Next() {
		var w models.WorkflowState
		if err := rows.Scan(&w.ID, &w.Name, &w.Category, &w.Position, &w.Color); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) GetState(ctx context.Context, wsID, id string) (models.WorkflowState, error) {
	var w models.WorkflowState
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, category, position, color FROM workflow_states WHERE id=$1 AND workspace_id=$2`, id, wsID).
		Scan(&w.ID, &w.Name, &w.Category, &w.Position, &w.Color)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// resolveStateTx returns a valid state id given an id and/or a name, within one
// workspace. If both are empty it defaults to the lowest-position state.
func (s *Store) resolveStateTx(ctx context.Context, tx pgx.Tx, wsID, id, name string) (string, error) {
	if id != "" {
		var got string
		err := tx.QueryRow(ctx,
			`SELECT id FROM workflow_states WHERE id=$1 AND workspace_id=$2`, id, wsID).Scan(&got)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("state id %q not found in this workspace", id)
		}
		return got, err
	}
	if name != "" {
		var got string
		err := tx.QueryRow(ctx,
			`SELECT id FROM workflow_states WHERE lower(name)=lower($1) AND workspace_id=$2`, name, wsID).Scan(&got)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("state name %q not found in this workspace", name)
		}
		return got, err
	}
	var got string
	err := tx.QueryRow(ctx,
		`SELECT id FROM workflow_states WHERE workspace_id=$1 ORDER BY position LIMIT 1`, wsID).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("no workflow states defined for this workspace")
	}
	return got, err
}

// ---- Label groups & labels ----

func (s *Store) ListLabelGroups(ctx context.Context, wsID string) ([]models.LabelGroup, error) {
	return s.ListLabelGroupsAcross(ctx, []string{wsID})
}

func (s *Store) ListLabelGroupsAcross(ctx context.Context, wsIDs []string) ([]models.LabelGroup, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, exclusive FROM label_groups WHERE workspace_id = ANY($1) ORDER BY name`, wsIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.LabelGroup
	for rows.Next() {
		var g models.LabelGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Exclusive); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) ListLabels(ctx context.Context, wsID string) ([]models.Label, error) {
	return s.ListLabelsAcross(ctx, []string{wsID})
}

func (s *Store) ListLabelsAcross(ctx context.Context, wsIDs []string) ([]models.Label, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, group_id, name, color FROM labels WHERE workspace_id = ANY($1) ORDER BY name`, wsIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Label
	for rows.Next() {
		var l models.Label
		if err := rows.Scan(&l.ID, &l.GroupID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CreateLabel creates (or returns existing) a label in one workspace,
// optionally in a named group.
func (s *Store) CreateLabel(ctx context.Context, wsID, name, color, groupName string) (models.Label, error) {
	var l models.Label
	var groupID *string
	if groupName != "" {
		var gid string
		err := s.pool.QueryRow(ctx,
			`INSERT INTO label_groups (workspace_id, name) VALUES ($1,$2)
			 ON CONFLICT (workspace_id, name) DO UPDATE SET name=EXCLUDED.name RETURNING id`,
			wsID, groupName).Scan(&gid)
		if err != nil {
			return l, err
		}
		groupID = &gid
	}
	if color == "" {
		color = "#94a3b8"
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO labels (workspace_id, group_id, name, color) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (workspace_id, name) DO UPDATE SET color=EXCLUDED.color, group_id=COALESCE(labels.group_id, EXCLUDED.group_id)
		 RETURNING id, group_id, name, color`,
		wsID, groupID, name, color).Scan(&l.ID, &l.GroupID, &l.Name, &l.Color)
	return l, err
}

// resolveLabelIDsTx turns a mix of label ids and label names into a concrete id
// set within one workspace, creating labels by name on the fly, and enforcing
// exclusive groups (keeping the last label seen for any exclusive group).
func (s *Store) resolveLabelIDsTx(ctx context.Context, tx pgx.Tx, wsID string, ids, names []string) ([]string, error) {
	type lbl struct {
		id      string
		groupID *string
	}
	resolved := []lbl{}

	for _, id := range ids {
		var got lbl
		err := tx.QueryRow(ctx,
			`SELECT id, group_id FROM labels WHERE id=$1 AND workspace_id=$2`, id, wsID).Scan(&got.id, &got.groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("label id %q not found in this workspace", id)
		}
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, got)
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var got lbl
		err := tx.QueryRow(ctx,
			`SELECT id, group_id FROM labels WHERE lower(name)=lower($1) AND workspace_id=$2`, name, wsID).
			Scan(&got.id, &got.groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			// auto-create ungrouped label in this workspace
			if err := tx.QueryRow(ctx,
				`INSERT INTO labels (workspace_id, name) VALUES ($1,$2) RETURNING id, group_id`,
				wsID, name).Scan(&got.id, &got.groupID); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		resolved = append(resolved, got)
	}

	// Enforce exclusive groups: last-wins per group.
	exclusive := map[string]bool{}
	if len(resolved) > 0 {
		grows, err := tx.Query(ctx,
			`SELECT id FROM label_groups WHERE exclusive=true AND workspace_id=$1`, wsID)
		if err != nil {
			return nil, err
		}
		for grows.Next() {
			var gid string
			if err := grows.Scan(&gid); err != nil {
				grows.Close()
				return nil, err
			}
			exclusive[gid] = true
		}
		grows.Close()
	}

	seenGroup := map[string]int{} // group id -> index in out
	out := []string{}
	for _, r := range resolved {
		if r.groupID != nil && exclusive[*r.groupID] {
			if idx, ok := seenGroup[*r.groupID]; ok {
				out[idx] = r.id // replace: last wins
				continue
			}
			seenGroup[*r.groupID] = len(out)
		}
		out = append(out, r.id)
	}
	return out, nil
}

// ---- Initiatives ----

func (s *Store) ListInitiatives(ctx context.Context, wsID string) ([]models.Initiative, error) {
	return s.ListInitiativesAcross(ctx, []string{wsID})
}

func (s *Store) ListInitiativesAcross(ctx context.Context, wsIDs []string) ([]models.Initiative, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description_md, status, position, repo_url, created_at, updated_at
		 FROM initiatives WHERE workspace_id = ANY($1) ORDER BY position, created_at`, wsIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Initiative
	for rows.Next() {
		var i models.Initiative
		if err := rows.Scan(&i.ID, &i.Name, &i.DescriptionMD, &i.Status, &i.Position, &i.RepoURL, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) GetInitiative(ctx context.Context, wsID, id string) (models.Initiative, error) {
	var i models.Initiative
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description_md, status, position, repo_url, created_at, updated_at
		 FROM initiatives WHERE id=$1 AND workspace_id=$2`, id, wsID).
		Scan(&i.ID, &i.Name, &i.DescriptionMD, &i.Status, &i.Position, &i.RepoURL, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}

func (s *Store) SaveInitiative(ctx context.Context, wsID string, i models.Initiative) (models.Initiative, error) {
	if i.Status == "" {
		i.Status = "active"
	}
	if i.ID == "" {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO initiatives (workspace_id, name, description_md, status, position, repo_url)
			 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at, updated_at`,
			wsID, i.Name, i.DescriptionMD, i.Status, i.Position, i.RepoURL).Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
		return i, err
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE initiatives SET name=$3, description_md=$4, status=$5, position=$6, repo_url=$7, updated_at=now()
		 WHERE id=$1 AND workspace_id=$2`,
		i.ID, wsID, i.Name, i.DescriptionMD, i.Status, i.Position, i.RepoURL)
	if err != nil {
		return i, err
	}
	if ct.RowsAffected() == 0 {
		return i, ErrNotFound
	}
	return s.GetInitiative(ctx, wsID, i.ID)
}

func (s *Store) DeleteInitiative(ctx context.Context, wsID, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM initiatives WHERE id=$1 AND workspace_id=$2`, id, wsID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Projects ----

func (s *Store) ListProjects(ctx context.Context, wsID, initiativeID string) ([]models.Project, error) {
	return s.ListProjectsAcross(ctx, []string{wsID}, initiativeID)
}

func (s *Store) ListProjectsAcross(ctx context.Context, wsIDs []string, initiativeID string) ([]models.Project, error) {
	q := `SELECT id, initiative_id, name, description_md, status, position, repo_url, created_at, updated_at
	      FROM projects WHERE workspace_id = ANY($1)`
	args := []any{wsIDs}
	if initiativeID != "" {
		q += ` AND initiative_id=$2`
		args = append(args, initiativeID)
	}
	q += ` ORDER BY position, created_at`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Project
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.InitiativeID, &p.Name, &p.DescriptionMD, &p.Status, &p.Position, &p.RepoURL, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, wsID, id string) (models.Project, error) {
	var p models.Project
	err := s.pool.QueryRow(ctx,
		`SELECT id, initiative_id, name, description_md, status, position, repo_url, created_at, updated_at
		 FROM projects WHERE id=$1 AND workspace_id=$2`, id, wsID).
		Scan(&p.ID, &p.InitiativeID, &p.Name, &p.DescriptionMD, &p.Status, &p.Position, &p.RepoURL, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) SaveProject(ctx context.Context, wsID string, p models.Project) (models.Project, error) {
	if p.Status == "" {
		p.Status = "active"
	}
	// An epic may only hang off an initiative in the same workspace.
	if p.InitiativeID != nil && *p.InitiativeID != "" {
		if _, err := s.GetInitiative(ctx, wsID, *p.InitiativeID); err != nil {
			return p, err
		}
	}
	if p.ID == "" {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO projects (workspace_id, initiative_id, name, description_md, status, position, repo_url)
			 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at, updated_at`,
			wsID, p.InitiativeID, p.Name, p.DescriptionMD, p.Status, p.Position, p.RepoURL).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
		return p, err
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE projects SET initiative_id=$3, name=$4, description_md=$5, status=$6, position=$7, repo_url=$8, updated_at=now()
		 WHERE id=$1 AND workspace_id=$2`,
		p.ID, wsID, p.InitiativeID, p.Name, p.DescriptionMD, p.Status, p.Position, p.RepoURL)
	if err != nil {
		return p, err
	}
	if ct.RowsAffected() == 0 {
		return p, ErrNotFound
	}
	return s.GetProject(ctx, wsID, p.ID)
}

func (s *Store) DeleteProject(ctx context.Context, wsID, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id=$1 AND workspace_id=$2`, id, wsID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
