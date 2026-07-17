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

func (s *Store) ListStates(ctx context.Context) ([]models.WorkflowState, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, category, position, color FROM workflow_states ORDER BY position`)
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

func (s *Store) GetState(ctx context.Context, id string) (models.WorkflowState, error) {
	var w models.WorkflowState
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, category, position, color FROM workflow_states WHERE id=$1`, id).
		Scan(&w.ID, &w.Name, &w.Category, &w.Position, &w.Color)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// resolveStateTx returns a valid state id given an id and/or a name.
// If both are empty it defaults to the lowest-position state (Triage/Backlog).
func (s *Store) resolveStateTx(ctx context.Context, tx pgx.Tx, id, name string) (string, error) {
	if id != "" {
		var got string
		err := tx.QueryRow(ctx, `SELECT id FROM workflow_states WHERE id=$1`, id).Scan(&got)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("state id %q not found", id)
		}
		return got, err
	}
	if name != "" {
		var got string
		err := tx.QueryRow(ctx, `SELECT id FROM workflow_states WHERE lower(name)=lower($1)`, name).Scan(&got)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("state name %q not found", name)
		}
		return got, err
	}
	var got string
	err := tx.QueryRow(ctx, `SELECT id FROM workflow_states ORDER BY position LIMIT 1`).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("no workflow states defined")
	}
	return got, err
}

// ---- Label groups & labels ----

func (s *Store) ListLabelGroups(ctx context.Context) ([]models.LabelGroup, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, exclusive FROM label_groups ORDER BY name`)
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

func (s *Store) ListLabels(ctx context.Context) ([]models.Label, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, group_id, name, color FROM labels ORDER BY name`)
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

// CreateLabel creates (or returns existing) a label, optionally in a named group.
func (s *Store) CreateLabel(ctx context.Context, name, color, groupName string) (models.Label, error) {
	var l models.Label
	var groupID *string
	if groupName != "" {
		var gid string
		err := s.pool.QueryRow(ctx,
			`INSERT INTO label_groups (name) VALUES ($1)
			 ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id`, groupName).Scan(&gid)
		if err != nil {
			return l, err
		}
		groupID = &gid
	}
	if color == "" {
		color = "#94a3b8"
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO labels (group_id, name, color) VALUES ($1,$2,$3)
		 ON CONFLICT (name) DO UPDATE SET color=EXCLUDED.color, group_id=COALESCE(labels.group_id, EXCLUDED.group_id)
		 RETURNING id, group_id, name, color`,
		groupID, name, color).Scan(&l.ID, &l.GroupID, &l.Name, &l.Color)
	return l, err
}

// resolveLabelIDsTx turns a mix of label ids and label names into a concrete id
// set, creating labels by name on the fly, and enforcing exclusive groups
// (keeping the last label seen for any exclusive group).
func (s *Store) resolveLabelIDsTx(ctx context.Context, tx pgx.Tx, ids, names []string) ([]string, error) {
	type lbl struct {
		id      string
		groupID *string
	}
	resolved := []lbl{}

	for _, id := range ids {
		var got lbl
		err := tx.QueryRow(ctx, `SELECT id, group_id FROM labels WHERE id=$1`, id).Scan(&got.id, &got.groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("label id %q not found", id)
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
		err := tx.QueryRow(ctx, `SELECT id, group_id FROM labels WHERE lower(name)=lower($1)`, name).Scan(&got.id, &got.groupID)
		if errors.Is(err, pgx.ErrNoRows) {
			// auto-create ungrouped label
			if err := tx.QueryRow(ctx,
				`INSERT INTO labels (name) VALUES ($1) RETURNING id, group_id`, name).Scan(&got.id, &got.groupID); err != nil {
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
		grows, err := tx.Query(ctx, `SELECT id FROM label_groups WHERE exclusive=true`)
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

func (s *Store) ListInitiatives(ctx context.Context) ([]models.Initiative, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description_md, status, position, repo_url, created_at, updated_at
		 FROM initiatives ORDER BY position, created_at`)
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

func (s *Store) GetInitiative(ctx context.Context, id string) (models.Initiative, error) {
	var i models.Initiative
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description_md, status, position, repo_url, created_at, updated_at FROM initiatives WHERE id=$1`, id).
		Scan(&i.ID, &i.Name, &i.DescriptionMD, &i.Status, &i.Position, &i.RepoURL, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, err
}

func (s *Store) SaveInitiative(ctx context.Context, i models.Initiative) (models.Initiative, error) {
	if i.Status == "" {
		i.Status = "active"
	}
	if i.ID == "" {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO initiatives (name, description_md, status, position, repo_url)
			 VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at, updated_at`,
			i.Name, i.DescriptionMD, i.Status, i.Position, i.RepoURL).Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
		return i, err
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE initiatives SET name=$2, description_md=$3, status=$4, position=$5, repo_url=$6, updated_at=now() WHERE id=$1`,
		i.ID, i.Name, i.DescriptionMD, i.Status, i.Position, i.RepoURL)
	if err != nil {
		return i, err
	}
	if ct.RowsAffected() == 0 {
		return i, ErrNotFound
	}
	return s.GetInitiative(ctx, i.ID)
}

func (s *Store) DeleteInitiative(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM initiatives WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Projects ----

func (s *Store) ListProjects(ctx context.Context, initiativeID string) ([]models.Project, error) {
	q := `SELECT id, initiative_id, name, description_md, status, position, repo_url, created_at, updated_at FROM projects`
	args := []any{}
	if initiativeID != "" {
		q += ` WHERE initiative_id=$1`
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

func (s *Store) GetProject(ctx context.Context, id string) (models.Project, error) {
	var p models.Project
	err := s.pool.QueryRow(ctx,
		`SELECT id, initiative_id, name, description_md, status, position, repo_url, created_at, updated_at FROM projects WHERE id=$1`, id).
		Scan(&p.ID, &p.InitiativeID, &p.Name, &p.DescriptionMD, &p.Status, &p.Position, &p.RepoURL, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) SaveProject(ctx context.Context, p models.Project) (models.Project, error) {
	if p.Status == "" {
		p.Status = "active"
	}
	if p.ID == "" {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO projects (initiative_id, name, description_md, status, position, repo_url)
			 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at, updated_at`,
			p.InitiativeID, p.Name, p.DescriptionMD, p.Status, p.Position, p.RepoURL).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
		return p, err
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE projects SET initiative_id=$2, name=$3, description_md=$4, status=$5, position=$6, repo_url=$7, updated_at=now() WHERE id=$1`,
		p.ID, p.InitiativeID, p.Name, p.DescriptionMD, p.Status, p.Position, p.RepoURL)
	if err != nil {
		return p, err
	}
	if ct.RowsAffected() == 0 {
		return p, ErrNotFound
	}
	return s.GetProject(ctx, p.ID)
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
