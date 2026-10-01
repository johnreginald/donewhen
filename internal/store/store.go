// Package store holds all Postgres persistence for Raenil.
//
// Every method that touches tenant data takes an explicit workspace id as its
// first argument. That is deliberate: the workspace is never read from a
// context inside this package, so forgetting to scope a query is a compile
// error rather than a silent cross-tenant leak.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"raenil/internal/models"
)

var ErrNotFound = errors.New("not found")

// ErrConflict means the write contradicts what is already recorded.
var ErrConflict = errors.New("conflict")

// ErrInvalid means the input itself is wrong; the message says how.
var ErrInvalid = errors.New("invalid")

// invalid builds an ErrInvalid with a message a caller can show as is.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
}

type Store struct {
	pool *pgxpool.Pool
	// reservedPrefix is the pre-workspace issue key prefix (RAENIL_ISSUE_PREFIX).
	// Legacy keys still carry it, so no workspace may claim it for new issues.
	reservedPrefix string
}

func New(pool *pgxpool.Pool, reservedPrefix string) *Store {
	if reservedPrefix == "" {
		reservedPrefix = "R"
	}
	return &Store{pool: pool, reservedPrefix: reservedPrefix}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ReservedPrefix is the issue key prefix no workspace may adopt.
func (s *Store) ReservedPrefix() string { return s.reservedPrefix }

// isUniqueViolation reports whether err is a duplicate-key error, optionally on
// a specific constraint.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// scopeIDs normalizes the one-or-many workspace scope into the array form the
// queries use, so a single-workspace read and a multi-workspace read share one
// code path and cannot drift apart.
func scopeIDs(one string, many []string) []string {
	if one != "" {
		return []string{one}
	}
	return many
}

// ---- Issues ----

type IssueFilter struct {
	// WorkspaceID scopes to exactly one workspace — what a browser request
	// always wants. WorkspaceIDs scopes to several, for an agent whose token
	// spans the account's memberships. Exactly one of the two must be set.
	WorkspaceID  string
	WorkspaceIDs []string
	StateID      string
	StateName    string // resolves by name, so it works across workspaces
	ProjectID    string
	InitiativeID string // all issues whose epic belongs to this Project (initiative)
	LabelID      string // all issues carrying this label
	LabelName    string // same, by name, so it works across workspaces
	Query        string
	ParentKey    string // list sub-issues of this epic key
	Limit        int
	// IncludeArchived keeps the issues of archived epics, which are hidden
	// from lists by default. Single-issue lookups are never filtered.
	IncludeArchived bool
	// NewestFirst orders by last update instead of board position, so a
	// capped list keeps the issues someone is most likely asking about.
	NewestFirst bool
}

const issueCols = `i.id, i.workspace_id, i.number, i.key, i.title, i.description_md, i.state_id,
	i.project_id, i.assignee_id, i.priority, i.position,
	(SELECT count(*) FROM documents d WHERE d.issue_id = i.id) AS doc_count,
	i.parent_key,
	(SELECT count(*) FROM issues c WHERE c.parent_key = i.key AND c.workspace_id = i.workspace_id) AS child_count,
	i.git_branch, i.pr_url,
	i.created_at, i.updated_at,
	(SELECT count(*) FROM issue_criteria c WHERE c.issue_id = i.id AND c.done) AS criteria_done,
	(SELECT count(*) FROM issue_criteria c WHERE c.issue_id = i.id) AS criteria_total,
	coalesce((SELECT a.actor FROM activity a WHERE a.issue_id = i.id ORDER BY a.created_at DESC LIMIT 1), '') AS last_actor`

// issueDest is where each of issueCols lands, in order. Every query that
// selects issueCols scans through this, so a new column is added in one place.
func issueDest(is *models.Issue) []any {
	return []any{&is.ID, &is.WorkspaceID, &is.Number, &is.Key, &is.Title, &is.DescriptionMD,
		&is.StateID, &is.ProjectID, &is.AssigneeID, &is.Priority, &is.Position,
		&is.DocCount, &is.ParentKey, &is.ChildCount, &is.GitBranch, &is.PRURL,
		&is.CreatedAt, &is.UpdatedAt, &is.CriteriaDone, &is.CriteriaTotal, &is.LastActor}
}

func scanIssue(row pgx.Row) (models.Issue, error) {
	var is models.Issue
	err := row.Scan(issueDest(&is)...)
	return is, err
}

func (s *Store) ListIssues(ctx context.Context, f IssueFilter) ([]models.Issue, error) {
	if f.WorkspaceID == "" && len(f.WorkspaceIDs) == 0 {
		return nil, errors.New("ListIssues: workspace id is required")
	}
	q := `SELECT ` + issueCols + ` FROM issues i WHERE i.workspace_id = ANY($1)`
	args := []any{scopeIDs(f.WorkspaceID, f.WorkspaceIDs)}
	n := 1
	add := func(cond string, v any) {
		n++
		q += fmt.Sprintf(" AND %s$%d", cond, n)
		args = append(args, v)
	}
	if !f.IncludeArchived {
		q += " AND (i.project_id IS NULL OR i.project_id NOT IN (SELECT id FROM projects WHERE status = 'archived'))"
	}
	if f.StateID != "" {
		add("i.state_id=", f.StateID)
	}
	if f.StateName != "" {
		n++
		q += fmt.Sprintf(" AND i.state_id IN (SELECT id FROM workflow_states WHERE lower(name)=lower($%d))", n)
		args = append(args, f.StateName)
	}
	if f.ProjectID != "" {
		add("i.project_id=", f.ProjectID)
	}
	if f.InitiativeID != "" {
		n++
		q += fmt.Sprintf(" AND i.project_id IN (SELECT id FROM projects WHERE initiative_id=$%d)", n)
		args = append(args, f.InitiativeID)
	}
	if f.LabelID != "" {
		n++
		q += fmt.Sprintf(" AND i.id IN (SELECT issue_id FROM issue_labels WHERE label_id=$%d)", n)
		args = append(args, f.LabelID)
	}
	if f.LabelName != "" {
		n++
		q += fmt.Sprintf(" AND i.id IN (SELECT il.issue_id FROM issue_labels il JOIN labels l ON l.id = il.label_id WHERE lower(l.name)=lower($%d))", n)
		args = append(args, f.LabelName)
	}
	if f.ParentKey != "" {
		add("i.parent_key=", f.ParentKey)
	}
	if f.Query != "" {
		n++
		q += fmt.Sprintf(" AND (i.title ILIKE $%d OR i.key ILIKE $%d)", n, n)
		args = append(args, "%"+f.Query+"%")
	}
	if f.NewestFirst {
		q += " ORDER BY i.updated_at DESC, i.number DESC"
	} else {
		q += " ORDER BY i.position ASC, i.number ASC"
	}
	if f.Limit > 0 {
		n++
		q += fmt.Sprintf(" LIMIT $%d", n)
		args = append(args, f.Limit)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Issue
	for rows.Next() {
		is, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, is)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachLabels(ctx, out)
}

func (s *Store) attachLabels(ctx context.Context, issues []models.Issue) ([]models.Issue, error) {
	if len(issues) == 0 {
		return issues, nil
	}
	idx := map[string]int{}
	ids := make([]string, len(issues))
	for i := range issues {
		issues[i].Labels = []models.Label{}
		idx[issues[i].ID] = i
		ids[i] = issues[i].ID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT il.issue_id, l.id, l.group_id, l.name, l.color
		FROM issue_labels il JOIN labels l ON l.id = il.label_id
		WHERE il.issue_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var issueID string
		var l models.Label
		if err := rows.Scan(&issueID, &l.ID, &l.GroupID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		if i, ok := idx[issueID]; ok {
			issues[i].Labels = append(issues[i].Labels, l)
		}
	}
	return issues, rows.Err()
}

// IssuesMissingDocs returns completed-category issues with no attached document.
func (s *Store) IssuesMissingDocs(ctx context.Context, wsIDs []string) ([]models.Issue, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+issueCols+`
		FROM issues i JOIN workflow_states w ON w.id = i.state_id
		WHERE i.workspace_id = ANY($1)
		  AND w.category = 'completed'
		  AND NOT EXISTS (SELECT 1 FROM documents d WHERE d.issue_id = i.id)
		ORDER BY i.number`, wsIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Issue
	for rows.Next() {
		is, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, is)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachLabels(ctx, out)
}

func (s *Store) GetIssue(ctx context.Context, wsID, id string) (models.Issue, error) {
	is, err := scanIssue(s.pool.QueryRow(ctx,
		`SELECT `+issueCols+` FROM issues i WHERE i.id=$1 AND i.workspace_id=$2`, id, wsID))
	if errors.Is(err, pgx.ErrNoRows) {
		return is, ErrNotFound
	}
	if err != nil {
		return is, err
	}
	out, err := s.attachLabels(ctx, []models.Issue{is})
	if err != nil {
		return is, err
	}
	return out[0], nil
}

// GetIssueByKey resolves an issue by its human key (e.g. "K-42"), case-insensitive.
func (s *Store) GetIssueByKey(ctx context.Context, wsID, key string) (models.Issue, error) {
	is, err := scanIssue(s.pool.QueryRow(ctx,
		`SELECT `+issueCols+` FROM issues i WHERE upper(i.key)=upper($1) AND i.workspace_id=$2`, key, wsID))
	if errors.Is(err, pgx.ErrNoRows) {
		return is, ErrNotFound
	}
	if err != nil {
		return is, err
	}
	out, err := s.attachLabels(ctx, []models.Issue{is})
	if err != nil {
		return is, err
	}
	return out[0], nil
}

// WorkspaceOfIssueKey finds which workspace owns a key, without reading the
// issue. Issue keys stay globally unique precisely so this is possible: it lets
// an unpinned caller name "R-8" and have the workspace resolved for them.
func (s *Store) WorkspaceOfIssueKey(ctx context.Context, key string) (string, error) {
	var wsID string
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id FROM issues WHERE upper(key)=upper($1)`, key).Scan(&wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return wsID, err
}

// WorkspaceOfIssueRef resolves a uuid or a human key to its workspace.
//
// This is the seam that lets a caller say "R-289" and have the workspace worked
// out rather than demanded. It is a lookup, not a guess — but it deliberately
// answers for ANY issue, so every caller must check membership on the result
// before using it.
func (s *Store) WorkspaceOfIssueRef(ctx context.Context, ref string) (string, error) {
	var wsID string
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id FROM issues WHERE id::text = $1 OR upper(key) = upper($1)`, ref).Scan(&wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return wsID, err
}

// WorkspaceOf resolves the owning workspace of a row in one of the tenant
// tables. Same contract as WorkspaceOfIssueRef: the caller checks membership.
func (s *Store) WorkspaceOf(ctx context.Context, table, id string) (string, error) {
	switch table {
	case "projects", "initiatives", "documents":
	default:
		return "", fmt.Errorf("WorkspaceOf: unsupported table %q", table)
	}
	var wsID string
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id FROM `+table+` WHERE id::text = $1`, id).Scan(&wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return wsID, err
}

type IssueInput struct {
	Title         string
	DescriptionMD string
	StateID       string
	StateName     string // optional: resolve state by name if StateID empty
	ProjectID     *string
	AssigneeID    *string
	Priority      int
	ParentKey     *string  // epic key this issue belongs under
	LabelIDs      []string // when non-nil, replaces the label set
	LabelNames    []string // optional: resolve/attach labels by name (exclusive-group aware)
}

// maxKeyAttempts bounds the retry loop that steps past a key already taken by a
// legacy (pre-workspace) issue.
const maxKeyAttempts = 5

func (s *Store) CreateIssue(ctx context.Context, wsID string, in IssueInput) (models.Issue, error) {
	if wsID == "" {
		return models.Issue{}, errors.New("CreateIssue: workspace id is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Issue{}, err
	}
	defer tx.Rollback(ctx)

	stateID, err := s.resolveStateTx(ctx, tx, wsID, in.StateID, in.StateName)
	if err != nil {
		return models.Issue{}, err
	}

	var id string
	// Bump the counters, then attempt the insert inside a savepoint. A rolled
	// back savepoint leaves the bump standing, so each retry moves forward
	// rather than re-proposing the key that just collided.
	for attempt := 0; ; attempt++ {
		var prefix string
		var seq, number int64
		if err := tx.QueryRow(ctx, `
			UPDATE workspaces SET issue_seq = issue_seq + 1, number_seq = number_seq + 1
			WHERE id = $1
			RETURNING key_prefix, issue_seq, number_seq`, wsID,
		).Scan(&prefix, &seq, &number); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.Issue{}, ErrNotFound
			}
			return models.Issue{}, err
		}
		key := fmt.Sprintf("%s-%d", prefix, seq)

		sp, err := tx.Begin(ctx)
		if err != nil {
			return models.Issue{}, err
		}
		err = sp.QueryRow(ctx, `
			INSERT INTO issues (workspace_id, number, key, title, description_md, state_id, project_id, assignee_id, priority, position, parent_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
			wsID, number, key, in.Title, in.DescriptionMD, stateID, in.ProjectID, in.AssigneeID,
			in.Priority, float64(number), in.ParentKey,
		).Scan(&id)
		if err == nil {
			if err := sp.Commit(ctx); err != nil {
				return models.Issue{}, err
			}
			break
		}
		_ = sp.Rollback(ctx)
		if !isUniqueViolation(err, "") || attempt >= maxKeyAttempts-1 {
			return models.Issue{}, err
		}
	}

	labelIDs, err := s.resolveLabelIDsTx(ctx, tx, wsID, in.LabelIDs, in.LabelNames)
	if err != nil {
		return models.Issue{}, err
	}
	if err := s.setLabelsTx(ctx, tx, id, labelIDs); err != nil {
		return models.Issue{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Issue{}, err
	}
	return s.GetIssue(ctx, wsID, id)
}

// IssuePatch carries optional updates; nil fields are left unchanged.
type IssuePatch struct {
	Title         *string
	DescriptionMD *string
	StateID       *string
	StateName     *string
	ProjectID     *string // pointer-to-pointer semantics handled by SetProject flag
	SetProject    bool
	AssigneeID    *string
	SetAssignee   bool
	Priority      *int
	Position      *float64
	ParentKey     *string // epic key; nil pointer + SetParent clears it
	SetParent     bool
	LabelIDs      []string // when non-nil, replaces label set
	LabelNames    []string
	ReplaceLabels bool
}

func (s *Store) UpdateIssue(ctx context.Context, wsID, id string, p IssuePatch) (models.Issue, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Issue{}, err
	}
	defer tx.Rollback(ctx)

	sets := []string{}
	args := []any{}
	n := 0
	set := func(col string, v any) {
		n++
		sets = append(sets, fmt.Sprintf("%s=$%d", col, n))
		args = append(args, v)
	}
	if p.Title != nil {
		set("title", *p.Title)
	}
	if p.DescriptionMD != nil {
		set("description_md", *p.DescriptionMD)
	}
	if p.StateID != nil || p.StateName != nil {
		sid := ""
		if p.StateID != nil {
			sid = *p.StateID
		}
		sn := ""
		if p.StateName != nil {
			sn = *p.StateName
		}
		resolved, err := s.resolveStateTx(ctx, tx, wsID, sid, sn)
		if err != nil {
			return models.Issue{}, err
		}
		set("state_id", resolved)
	}
	if p.SetProject {
		set("project_id", p.ProjectID)
	}
	if p.SetParent {
		set("parent_key", p.ParentKey)
	}
	if p.SetAssignee {
		set("assignee_id", p.AssigneeID)
	}
	if p.Priority != nil {
		set("priority", *p.Priority)
	}
	if p.Position != nil {
		set("position", *p.Position)
	}

	if len(sets) > 0 {
		sets = append(sets, "updated_at=now()")
		n++
		q := fmt.Sprintf("UPDATE issues SET %s WHERE id=$%d", strings.Join(sets, ", "), n)
		args = append(args, id)
		n++
		q += fmt.Sprintf(" AND workspace_id=$%d", n)
		args = append(args, wsID)
		ct, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return models.Issue{}, err
		}
		if ct.RowsAffected() == 0 {
			return models.Issue{}, ErrNotFound
		}
	}

	if p.ReplaceLabels {
		// Guard the label-only path: without a SET clause above, nothing has
		// yet proved this issue belongs to the caller's workspace.
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM issues WHERE id=$1 AND workspace_id=$2)`, id, wsID).Scan(&ok); err != nil {
			return models.Issue{}, err
		}
		if !ok {
			return models.Issue{}, ErrNotFound
		}
		labelIDs, err := s.resolveLabelIDsTx(ctx, tx, wsID, p.LabelIDs, p.LabelNames)
		if err != nil {
			return models.Issue{}, err
		}
		if err := s.setLabelsTx(ctx, tx, id, labelIDs); err != nil {
			return models.Issue{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Issue{}, err
	}
	return s.GetIssue(ctx, wsID, id)
}

func (s *Store) DeleteIssue(ctx context.Context, wsID, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM issues WHERE id=$1 AND workspace_id=$2`, id, wsID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) setLabelsTx(ctx context.Context, tx pgx.Tx, issueID string, labelIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM issue_labels WHERE issue_id=$1`, issueID); err != nil {
		return err
	}
	for _, lid := range labelIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO issue_labels (issue_id, label_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			issueID, lid); err != nil {
			return err
		}
	}
	return nil
}
