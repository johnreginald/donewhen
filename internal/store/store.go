// Package store holds all Postgres persistence for Raenil.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"raenil/internal/models"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool   *pgxpool.Pool
	prefix string // issue key prefix, e.g. "K"
}

func New(pool *pgxpool.Pool, prefix string) *Store {
	if prefix == "" {
		prefix = "R"
	}
	return &Store{pool: pool, prefix: prefix}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ---- Issues ----

type IssueFilter struct {
	StateID   string
	ProjectID string
	Query     string
	Limit     int
}

const issueCols = `i.id, i.number, i.key, i.title, i.description_md, i.state_id,
	i.project_id, i.assignee_id, i.priority, i.position,
	(SELECT count(*) FROM documents d WHERE d.issue_id = i.id) AS doc_count,
	i.created_at, i.updated_at`

func scanIssue(row pgx.Row) (models.Issue, error) {
	var is models.Issue
	err := row.Scan(&is.ID, &is.Number, &is.Key, &is.Title, &is.DescriptionMD,
		&is.StateID, &is.ProjectID, &is.AssigneeID, &is.Priority, &is.Position,
		&is.DocCount, &is.CreatedAt, &is.UpdatedAt)
	return is, err
}

func (s *Store) ListIssues(ctx context.Context, f IssueFilter) ([]models.Issue, error) {
	q := `SELECT ` + issueCols + ` FROM issues i WHERE 1=1`
	args := []any{}
	n := 0
	add := func(cond string, v any) {
		n++
		q += fmt.Sprintf(" AND %s$%d", cond, n)
		args = append(args, v)
	}
	if f.StateID != "" {
		add("i.state_id=", f.StateID)
	}
	if f.ProjectID != "" {
		add("i.project_id=", f.ProjectID)
	}
	if f.Query != "" {
		n++
		q += fmt.Sprintf(" AND (i.title ILIKE $%d OR i.key ILIKE $%d)", n, n)
		args = append(args, "%"+f.Query+"%")
	}
	q += " ORDER BY i.position ASC, i.number ASC"
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
func (s *Store) IssuesMissingDocs(ctx context.Context) ([]models.Issue, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+issueCols+`
		FROM issues i JOIN workflow_states w ON w.id = i.state_id
		WHERE w.category = 'completed'
		  AND NOT EXISTS (SELECT 1 FROM documents d WHERE d.issue_id = i.id)
		ORDER BY i.number`)
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

func (s *Store) GetIssue(ctx context.Context, id string) (models.Issue, error) {
	is, err := scanIssue(s.pool.QueryRow(ctx,
		`SELECT `+issueCols+` FROM issues i WHERE i.id=$1`, id))
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
func (s *Store) GetIssueByKey(ctx context.Context, key string) (models.Issue, error) {
	is, err := scanIssue(s.pool.QueryRow(ctx,
		`SELECT `+issueCols+` FROM issues i WHERE upper(i.key)=upper($1)`, key))
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

type IssueInput struct {
	Title         string
	DescriptionMD string
	StateID       string
	StateName     string   // optional: resolve state by name if StateID empty
	ProjectID     *string
	AssigneeID    *string
	Priority      int
	LabelIDs      []string // when non-nil, replaces the label set
	LabelNames    []string // optional: resolve/attach labels by name (exclusive-group aware)
}

func (s *Store) CreateIssue(ctx context.Context, in IssueInput) (models.Issue, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Issue{}, err
	}
	defer tx.Rollback(ctx)

	stateID, err := s.resolveStateTx(ctx, tx, in.StateID, in.StateName)
	if err != nil {
		return models.Issue{}, err
	}

	var number int
	if err := tx.QueryRow(ctx, `SELECT nextval('issue_number_seq')`).Scan(&number); err != nil {
		return models.Issue{}, err
	}
	key := fmt.Sprintf("%s-%d", s.prefix, number)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO issues (number, key, title, description_md, state_id, project_id, assignee_id, priority, position)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		number, key, in.Title, in.DescriptionMD, stateID, in.ProjectID, in.AssigneeID, in.Priority, float64(number),
	).Scan(&id)
	if err != nil {
		return models.Issue{}, err
	}

	labelIDs, err := s.resolveLabelIDsTx(ctx, tx, in.LabelIDs, in.LabelNames)
	if err != nil {
		return models.Issue{}, err
	}
	if err := s.setLabelsTx(ctx, tx, id, labelIDs); err != nil {
		return models.Issue{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Issue{}, err
	}
	return s.GetIssue(ctx, id)
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
	LabelIDs      []string // when non-nil, replaces label set
	LabelNames    []string
	ReplaceLabels bool
}

func (s *Store) UpdateIssue(ctx context.Context, id string, p IssuePatch) (models.Issue, error) {
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
		resolved, err := s.resolveStateTx(ctx, tx, sid, sn)
		if err != nil {
			return models.Issue{}, err
		}
		set("state_id", resolved)
	}
	if p.SetProject {
		set("project_id", p.ProjectID)
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
		ct, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return models.Issue{}, err
		}
		if ct.RowsAffected() == 0 {
			return models.Issue{}, ErrNotFound
		}
	}

	if p.ReplaceLabels {
		labelIDs, err := s.resolveLabelIDsTx(ctx, tx, p.LabelIDs, p.LabelNames)
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
	return s.GetIssue(ctx, id)
}

func (s *Store) DeleteIssue(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM issues WHERE id=$1`, id)
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
