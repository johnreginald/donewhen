package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- Comments ----

func (s *Store) ListComments(ctx context.Context, issueID string) ([]models.Comment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, issue_id, body_md, actor, created_at FROM comments WHERE issue_id=$1 ORDER BY created_at`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.BodyMD, &c.Actor, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateComment(ctx context.Context, issueID, body, actor string) (models.Comment, error) {
	if actor == "" {
		actor = "human"
	}
	var c models.Comment
	err := s.pool.QueryRow(ctx,
		`INSERT INTO comments (issue_id, body_md, actor) VALUES ($1,$2,$3)
		 RETURNING id, issue_id, body_md, actor, created_at`,
		issueID, body, actor).Scan(&c.ID, &c.IssueID, &c.BodyMD, &c.Actor, &c.CreatedAt)
	return c, err
}

// ---- Documents ----

// DocFilter narrows a document listing to one attach target.
type DocFilter struct {
	ProjectID    string
	IssueID      string
	InitiativeID string
}

const docCols = `id, title, body_md, type, author, project_id, initiative_id, issue_id, created_at, updated_at`

func scanDocument(row pgx.Row) (models.Document, error) {
	var d models.Document
	err := row.Scan(&d.ID, &d.Title, &d.BodyMD, &d.Type, &d.Author, &d.ProjectID, &d.InitiativeID, &d.IssueID,
		&d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *Store) ListDocuments(ctx context.Context, f DocFilter) ([]models.Document, error) {
	q := `SELECT ` + docCols + ` FROM documents WHERE 1=1`
	args := []any{}
	n := 0
	add := func(col string, v string) {
		if v == "" {
			return
		}
		n++
		q += fmt.Sprintf(" AND %s=$%d", col, n)
		args = append(args, v)
	}
	add("project_id", f.ProjectID)
	add("issue_id", f.IssueID)
	add("initiative_id", f.InitiativeID)
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachDocLabels(ctx, out)
}

func (s *Store) attachDocLabels(ctx context.Context, docs []models.Document) ([]models.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	idx := map[string]int{}
	ids := make([]string, len(docs))
	for i := range docs {
		docs[i].Labels = []models.Label{}
		idx[docs[i].ID] = i
		ids[i] = docs[i].ID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT dl.document_id, l.id, l.group_id, l.name, l.color
		FROM document_labels dl JOIN labels l ON l.id = dl.label_id
		WHERE dl.document_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var docID string
		var l models.Label
		if err := rows.Scan(&docID, &l.ID, &l.GroupID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		if i, ok := idx[docID]; ok {
			docs[i].Labels = append(docs[i].Labels, l)
		}
	}
	return docs, rows.Err()
}

func (s *Store) GetDocument(ctx context.Context, id string) (models.Document, error) {
	d, err := scanDocument(s.pool.QueryRow(ctx, `SELECT `+docCols+` FROM documents WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	out, err := s.attachDocLabels(ctx, []models.Document{d})
	if err != nil {
		return d, err
	}
	return out[0], nil
}

func (s *Store) SaveDocument(ctx context.Context, d models.Document) (models.Document, error) {
	if d.Type == "" {
		d.Type = "reference"
	}
	if d.ID == "" {
		if d.Author == "" {
			d.Author = "human"
		}
		err := s.pool.QueryRow(ctx,
			`INSERT INTO documents (title, body_md, type, author, project_id, initiative_id, issue_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at, updated_at`,
			d.Title, d.BodyMD, d.Type, d.Author, d.ProjectID, d.InitiativeID, d.IssueID).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			return d, err
		}
		return s.GetDocument(ctx, d.ID)
	}
	// Update leaves author (provenance) immutable.
	ct, err := s.pool.Exec(ctx,
		`UPDATE documents SET title=$2, body_md=$3, type=$4, project_id=$5, initiative_id=$6, issue_id=$7, updated_at=now() WHERE id=$1`,
		d.ID, d.Title, d.BodyMD, d.Type, d.ProjectID, d.InitiativeID, d.IssueID)
	if err != nil {
		return d, err
	}
	if ct.RowsAffected() == 0 {
		return d, ErrNotFound
	}
	return s.GetDocument(ctx, d.ID)
}

// SetDocumentLabels replaces a document's label set (exclusive-group aware).
func (s *Store) SetDocumentLabels(ctx context.Context, docID string, ids, names []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	labelIDs, err := s.resolveLabelIDsTx(ctx, tx, ids, names)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM document_labels WHERE document_id=$1`, docID); err != nil {
		return err
	}
	for _, lid := range labelIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_labels (document_id, label_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			docID, lid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteDocument(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
