package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ErrNotMember is returned when a user has no membership in a workspace. It is
// distinct from ErrNotFound so callers can answer 403 rather than 404 for a
// workspace that demonstrably exists.
var ErrNotMember = errors.New("not a member of this workspace")

const workspaceCols = `id, slug, name, key_prefix, position, created_at, updated_at`

func scanWorkspace(row pgx.Row) (models.Workspace, error) {
	var w models.Workspace
	err := row.Scan(&w.ID, &w.Slug, &w.Name, &w.KeyPrefix, &w.Position, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// ---- lookup ----

func (s *Store) GetWorkspace(ctx context.Context, id string) (models.Workspace, error) {
	w, err := scanWorkspace(s.pool.QueryRow(ctx,
		`SELECT `+workspaceCols+` FROM workspaces WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// ResolveWorkspace accepts either a uuid or a slug, so callers can say
// "lumos" as readily as the id.
func (s *Store) ResolveWorkspace(ctx context.Context, ref string) (models.Workspace, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return models.Workspace{}, ErrNotFound
	}
	w, err := scanWorkspace(s.pool.QueryRow(ctx,
		`SELECT `+workspaceCols+` FROM workspaces
		  WHERE id::text = $1 OR lower(slug) = lower($1) OR upper(key_prefix) = upper($1)`, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	return w, err
}

// ListWorkspaces returns every workspace, for CLI and admin paths only. Request
// handling must use ListMemberships instead.
func (s *Store) ListWorkspaces(ctx context.Context) ([]models.Workspace, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+workspaceCols+` FROM workspaces ORDER BY position, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Workspace{}
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ---- membership ----

// ListMemberships returns the workspaces a user belongs to, with their role.
// This is the only workspace listing a request handler should use.
func (s *Store) ListMemberships(ctx context.Context, userID string) ([]models.Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT w.id, w.slug, w.name, w.key_prefix, w.position, w.created_at, w.updated_at, m.role
		FROM workspaces w
		JOIN workspace_members m ON m.workspace_id = w.id
		WHERE m.user_id = $1
		ORDER BY w.position, w.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Membership{}
	for rows.Next() {
		var m models.Membership
		if err := rows.Scan(&m.ID, &m.Slug, &m.Name, &m.KeyPrefix, &m.Position,
			&m.CreatedAt, &m.UpdatedAt, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RoleIn returns the user's role in a workspace, or ErrNotMember. This is the
// single gate every request passes through before any scoped query runs.
func (s *Store) RoleIn(ctx context.Context, wsID, userID string) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, wsID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotMember
	}
	return role, err
}

// ListMembers returns everyone with access to a workspace.
func (s *Store) ListMembers(ctx context.Context, wsID string) ([]models.Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, m.role, m.created_at
		FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1
		ORDER BY m.created_at`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Member{}
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMember grants a user access to a workspace (idempotent: re-adding updates
// the role).
func (s *Store) AddMember(ctx context.Context, wsID, userID, role string) error {
	if role == "" {
		role = models.RoleMember
	}
	if role != models.RoleOwner && role != models.RoleAdmin && role != models.RoleMember {
		return fmt.Errorf("unknown role %q", role)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role`, wsID, userID, role)
	return err
}

// RemoveMember revokes access, refusing to strand a workspace with no owner.
func (s *Store) RemoveMember(ctx context.Context, wsID, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var role string
	err = tx.QueryRow(ctx,
		`SELECT role FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, wsID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == models.RoleOwner {
		var owners int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM workspace_members WHERE workspace_id=$1 AND role='owner'`, wsID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return errors.New("cannot remove the last owner of a workspace")
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, wsID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---- create / update ----

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)
var prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// Slugify turns a display name into a url-safe handle.
func Slugify(name string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// ValidatePrefix enforces the shape of an issue key prefix and refuses the one
// reserved for pre-workspace keys.
func ValidatePrefix(prefix, reserved string) error {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if prefix == "" {
		return errors.New("key prefix is required")
	}
	if !prefixRe.MatchString(prefix) {
		return errors.New("key prefix must be letters and digits, starting with a letter")
	}
	if reserved != "" && prefix == strings.ToUpper(reserved) {
		return fmt.Errorf("key prefix %q is reserved for pre-workspace issue keys", prefix)
	}
	return nil
}

// CreateWorkspace makes a workspace and seeds its counters so a fresh workspace
// can never mint a key or number that an existing issue already holds.
func (s *Store) CreateWorkspace(ctx context.Context, name, slug, prefix string, ownerID string) (models.Workspace, error) {
	if slug == "" {
		slug = Slugify(name)
	}
	prefix = strings.ToUpper(strings.TrimSpace(prefix))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Workspace{}, err
	}
	defer tx.Rollback(ctx)

	// A brand new workspace holds no issues, so number_seq starts at zero; but
	// the prefix may still collide with legacy keys, so issue_seq starts past
	// the highest suffix already using it.
	var seq int64
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(max(coalesce(nullif(regexp_replace(split_part(key,'-',2), '[^0-9]', '', 'g'), ''), '0')::bigint), 0)
		  FROM issues WHERE upper(split_part(key,'-',1)) = $1`, prefix).Scan(&seq); err != nil {
		return models.Workspace{}, err
	}

	w, err := scanWorkspace(tx.QueryRow(ctx, `
		INSERT INTO workspaces (slug, name, key_prefix, issue_seq, position)
		VALUES ($1,$2,$3,$4, coalesce((SELECT max(position)+1 FROM workspaces), 0))
		RETURNING `+workspaceCols, slug, name, prefix, seq))
	if err != nil {
		return models.Workspace{}, err
	}

	if ownerID != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1,$2,'owner')
			 ON CONFLICT DO NOTHING`, w.ID, ownerID); err != nil {
			return models.Workspace{}, err
		}
	}

	// Every workspace needs its own board columns and label taxonomy; without
	// them a new workspace cannot hold an issue at all.
	if _, err := tx.Exec(ctx, `
		INSERT INTO workflow_states (workspace_id, name, category, position, color) VALUES
			($1,'Triage','triage',0,'#8A8F9C'),
			($1,'Backlog','backlog',1,'#7C8698'),
			($1,'Aligning','unstarted',2,'#A78BFA'),
			($1,'Ready','unstarted',3,'#38BDF8'),
			($1,'In Progress','started',4,'#FBBF24'),
			($1,'In Review','started',5,'#FB923C'),
			($1,'Done','completed',6,'#34D399'),
			($1,'Canceled','canceled',7,'#6B7280')`, w.ID); err != nil {
		return models.Workspace{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO label_groups (workspace_id, name, exclusive) VALUES
			($1,'repo',true),($1,'platform',true),($1,'type',true),
			($1,'domain',true),($1,'triage',true)`, w.ID); err != nil {
		return models.Workspace{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO labels (workspace_id, group_id, name, color)
		SELECT $1, g.id, v.name, v.color
		FROM (VALUES
			('bug','#F87171','type'),('feature','#34D399','type'),
			('chore','#94A3B8','type'),('tech-debt','#FBBF24','type'),
			('needs-triage','#a1a1aa','triage'),('needs-info','#fbbf24','triage'),
			('ready-for-agent','#38bdf8','triage'),('ready-for-human','#c084fc','triage'),
			('wontfix','#f87171','triage')
		) AS v(name,color,grp)
		JOIN label_groups g ON g.workspace_id = $1 AND g.name = v.grp`, w.ID); err != nil {
		return models.Workspace{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Workspace{}, err
	}
	return w, nil
}

// UpdateWorkspace renames a workspace and/or changes its key prefix. A prefix
// change also lifts issue_seq past anything already using the new prefix.
func (s *Store) UpdateWorkspace(ctx context.Context, id string, name, slug, prefix *string) (models.Workspace, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Workspace{}, err
	}
	defer tx.Rollback(ctx)

	sets := []string{"updated_at=now()"}
	args := []any{id}
	n := 1
	if name != nil {
		n++
		sets = append(sets, fmt.Sprintf("name=$%d", n))
		args = append(args, *name)
	}
	if slug != nil {
		n++
		sets = append(sets, fmt.Sprintf("slug=$%d", n))
		args = append(args, Slugify(*slug))
	}
	if prefix != nil {
		p := strings.ToUpper(strings.TrimSpace(*prefix))
		n++
		sets = append(sets, fmt.Sprintf("key_prefix=$%d", n))
		args = append(args, p)
		n++
		sets = append(sets, fmt.Sprintf(`issue_seq=GREATEST(issue_seq, coalesce((
			SELECT max(coalesce(nullif(regexp_replace(split_part(key,'-',2), '[^0-9]', '', 'g'), ''), '0')::bigint)
			  FROM issues WHERE upper(split_part(key,'-',1)) = $%d), 0))`, n))
		args = append(args, p)
	}

	ct, err := tx.Exec(ctx,
		fmt.Sprintf(`UPDATE workspaces SET %s WHERE id=$1`, strings.Join(sets, ", ")), args...)
	if err != nil {
		return models.Workspace{}, err
	}
	if ct.RowsAffected() == 0 {
		return models.Workspace{}, ErrNotFound
	}
	w, err := scanWorkspace(tx.QueryRow(ctx, `SELECT `+workspaceCols+` FROM workspaces WHERE id=$1`, id))
	if err != nil {
		return w, err
	}
	return w, tx.Commit(ctx)
}

// ---- the user's active workspace ----

// LastWorkspace returns the workspace the user was last in, if they are still a
// member of it.
func (s *Store) LastWorkspace(ctx context.Context, userID string) (string, error) {
	var id *string
	err := s.pool.QueryRow(ctx, `
		SELECT u.last_workspace_id FROM users u
		JOIN workspace_members m ON m.workspace_id = u.last_workspace_id AND m.user_id = u.id
		WHERE u.id = $1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) || id == nil {
		return "", ErrNotFound
	}
	return *id, err
}

// SetLastWorkspace remembers where the user was, so a fresh session lands there.
func (s *Store) SetLastWorkspace(ctx context.Context, userID, wsID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET last_workspace_id=$2 WHERE id=$1`, userID, wsID)
	return err
}
