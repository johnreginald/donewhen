package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"kanri/internal/models"
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

func (s *Store) ListDocuments(ctx context.Context, projectID string) ([]models.Document, error) {
	q := `SELECT id, title, body_md, project_id, created_at, updated_at FROM documents`
	args := []any{}
	if projectID != "" {
		q += ` WHERE project_id=$1`
		args = append(args, projectID)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Document
	for rows.Next() {
		var d models.Document
		if err := rows.Scan(&d.ID, &d.Title, &d.BodyMD, &d.ProjectID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDocument(ctx context.Context, id string) (models.Document, error) {
	var d models.Document
	err := s.pool.QueryRow(ctx,
		`SELECT id, title, body_md, project_id, created_at, updated_at FROM documents WHERE id=$1`, id).
		Scan(&d.ID, &d.Title, &d.BodyMD, &d.ProjectID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *Store) SaveDocument(ctx context.Context, d models.Document) (models.Document, error) {
	if d.ID == "" {
		err := s.pool.QueryRow(ctx,
			`INSERT INTO documents (title, body_md, project_id) VALUES ($1,$2,$3)
			 RETURNING id, created_at, updated_at`,
			d.Title, d.BodyMD, d.ProjectID).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
		return d, err
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE documents SET title=$2, body_md=$3, project_id=$4, updated_at=now() WHERE id=$1`,
		d.ID, d.Title, d.BodyMD, d.ProjectID)
	if err != nil {
		return d, err
	}
	if ct.RowsAffected() == 0 {
		return d, ErrNotFound
	}
	return s.GetDocument(ctx, d.ID)
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
