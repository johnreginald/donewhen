package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/johnreginald/donewhen/internal/models"
)

// ---- Comments ----

// ListComments returns an issue's comments. The issue is resolved through the
// workspace first, so a caller cannot read a conversation it cannot see.
func (s *Store) ListComments(ctx context.Context, wsID, issueID string) ([]models.Comment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.issue_id, c.body_md, c.actor, c.kind, c.created_at
		FROM comments c JOIN issues i ON i.id = c.issue_id
		WHERE c.issue_id=$1 AND i.workspace_id=$2
		ORDER BY c.created_at`, issueID, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.BodyMD, &c.Actor, &c.Kind, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateComment(ctx context.Context, wsID, issueID, body, actor string) (models.Comment, error) {
	if actor == "" {
		actor = "human"
	}
	var c models.Comment
	err := s.pool.QueryRow(ctx,
		`INSERT INTO comments (issue_id, body_md, actor)
		 SELECT $1,$2,$3 FROM issues WHERE id=$1 AND workspace_id=$4
		 RETURNING id, issue_id, body_md, actor, kind, created_at`,
		issueID, body, actor, wsID).Scan(&c.ID, &c.IssueID, &c.BodyMD, &c.Actor, &c.Kind, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// ---- Documents ----

// DocFilter narrows a document listing to one attach target.
type DocFilter struct {
	ProjectID    string
	IssueID      string
	InitiativeID string
	// WorkspaceIDs widens a listing beyond the single workspace passed to
	// ListDocuments — the agent surface only.
	WorkspaceIDs []string
}

const docCols = `id, title, body_md, type, author, project_id, initiative_id, issue_id, created_at, updated_at`

func scanDocument(row pgx.Row) (models.Document, error) {
	var d models.Document
	err := row.Scan(&d.ID, &d.Title, &d.BodyMD, &d.Type, &d.Author, &d.ProjectID, &d.InitiativeID, &d.IssueID,
		&d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *Store) ListDocuments(ctx context.Context, wsID string, f DocFilter) ([]models.Document, error) {
	scope := f.WorkspaceIDs
	if len(scope) == 0 {
		scope = []string{wsID}
	}
	q := `SELECT ` + docCols + ` FROM documents WHERE workspace_id = ANY($1)`
	args := []any{scope}
	n := 1
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

func (s *Store) GetDocument(ctx context.Context, wsID, id string) (models.Document, error) {
	d, err := scanDocument(s.pool.QueryRow(ctx,
		`SELECT `+docCols+` FROM documents WHERE id=$1 AND workspace_id=$2`, id, wsID))
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

// DocLabels is an optional label replacement carried with a document write, so
// the document and its labels commit together. The zero value leaves labels alone.
type DocLabels struct {
	Set   bool
	IDs   []string
	Names []string
}

// checkDocRefsTx rejects a document that points at an issue, epic or initiative
// outside wsID (or that does not exist). nil and empty references are skipped.
func (s *Store) checkDocRefsTx(ctx context.Context, tx pgx.Tx, wsID string, issueID, projectID, initiativeID *string) error {
	for _, c := range []struct {
		id    *string
		query string
	}{
		{issueID, `SELECT EXISTS(SELECT 1 FROM issues WHERE id::text=$1 AND workspace_id=$2)`},
		{projectID, `SELECT EXISTS(SELECT 1 FROM projects WHERE id::text=$1 AND workspace_id=$2)`},
		{initiativeID, `SELECT EXISTS(SELECT 1 FROM initiatives WHERE id::text=$1 AND workspace_id=$2)`},
	} {
		if c.id == nil || *c.id == "" {
			continue
		}
		var ok bool
		if err := tx.QueryRow(ctx, c.query, *c.id, wsID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) setDocLabelsTx(ctx context.Context, tx pgx.Tx, wsID, docID string, l DocLabels) error {
	if !l.Set {
		return nil
	}
	labelIDs, err := s.resolveLabelIDsTx(ctx, tx, wsID, l.IDs, l.Names)
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
	return nil
}

// SaveDocument creates a document. Changing one goes through UpdateDocument,
// which only touches the fields it is given.
func (s *Store) SaveDocument(ctx context.Context, wsID string, d models.Document) (models.Document, error) {
	return s.CreateDocument(ctx, wsID, d, DocLabels{})
}

// CreateDocument creates a document and its labels in one transaction: a label
// error leaves nothing saved.
func (s *Store) CreateDocument(ctx context.Context, wsID string, d models.Document, labels DocLabels) (models.Document, error) {
	if d.ID != "" {
		return d, invalid("SaveDocument creates; use UpdateDocument to change one")
	}
	if d.Type == "" {
		d.Type = "reference"
	}
	if d.Author == "" {
		d.Author = "human"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	// A document may only attach to things inside its own workspace.
	if err := s.checkDocRefsTx(ctx, tx, wsID, d.IssueID, d.ProjectID, d.InitiativeID); err != nil {
		return d, err
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO documents (workspace_id, title, body_md, type, author, project_id, initiative_id, issue_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at, updated_at`,
		wsID, d.Title, d.BodyMD, d.Type, d.Author, nilIfEmpty(d.ProjectID), nilIfEmpty(d.InitiativeID), nilIfEmpty(d.IssueID)).
		Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, err
	}
	if err := s.setDocLabelsTx(ctx, tx, wsID, d.ID, labels); err != nil {
		return d, err
	}
	// Timeline: an artifact was written for this issue.
	if d.IssueID != nil && *d.IssueID != "" {
		is, err := s.getIssueTx(ctx, tx, wsID, *d.IssueID, false)
		if err != nil {
			return d, err
		}
		if err := insertActivity(ctx, tx, wsID, models.Activity{
			IssueID: d.IssueID, IssueKey: is.Key, IssueTitle: is.Title,
			Actor: d.Author, Kind: "artifact_written", Detail: d.Title,
		}); err != nil {
			return d, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return d, err
	}
	return s.GetDocument(ctx, wsID, d.ID)
}

// nilIfEmpty turns a pointer to "" into nil, so an empty reference is stored as NULL.
func nilIfEmpty(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	return p
}

// DocumentPatch carries optional updates; nil fields are left unchanged. An empty
// IssueID, ProjectID or InitiativeID detaches that link. Labels, when Set,
// replace the label set in the same transaction as the document.
type DocumentPatch struct {
	Title        *string
	BodyMD       *string
	Type         *string
	IssueID      *string
	ProjectID    *string
	InitiativeID *string
	Labels       DocLabels
}

// UpdateDocument applies a patch to one document: it loads the row under a
// lock, overlays only the fields sent, and writes the document and its labels in
// one transaction. Author (provenance) never changes.
func (s *Store) UpdateDocument(ctx context.Context, wsID, id string, p DocumentPatch) (models.Document, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Document{}, err
	}
	defer tx.Rollback(ctx)
	d, err := scanDocument(tx.QueryRow(ctx,
		`SELECT `+docCols+` FROM documents WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, id, wsID))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	if p.Title != nil {
		d.Title = *p.Title
	}
	if p.BodyMD != nil {
		d.BodyMD = *p.BodyMD
	}
	if p.Type != nil && *p.Type != "" {
		d.Type = *p.Type
	}
	if p.IssueID != nil {
		d.IssueID = nilIfEmpty(p.IssueID)
	}
	if p.ProjectID != nil {
		d.ProjectID = nilIfEmpty(p.ProjectID)
	}
	if p.InitiativeID != nil {
		d.InitiativeID = nilIfEmpty(p.InitiativeID)
	}
	// Only newly sent links need proving; the stored ones were checked when set.
	if err := s.checkDocRefsTx(ctx, tx, wsID, nilIfEmpty(p.IssueID), nilIfEmpty(p.ProjectID), nilIfEmpty(p.InitiativeID)); err != nil {
		return d, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE documents SET title=$3, body_md=$4, type=$5, project_id=$6, initiative_id=$7, issue_id=$8, updated_at=now()
		 WHERE id=$1 AND workspace_id=$2`,
		id, wsID, d.Title, d.BodyMD, d.Type, d.ProjectID, d.InitiativeID, d.IssueID); err != nil {
		return d, err
	}
	if err := s.setDocLabelsTx(ctx, tx, wsID, id, p.Labels); err != nil {
		return d, err
	}
	if err := tx.Commit(ctx); err != nil {
		return d, err
	}
	return s.GetDocument(ctx, wsID, id)
}

// SetDocumentLabels replaces a document's label set (exclusive-group aware).
func (s *Store) SetDocumentLabels(ctx context.Context, wsID, docID string, ids, names []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var ok bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM documents WHERE id=$1 AND workspace_id=$2)`, docID, wsID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	labelIDs, err := s.resolveLabelIDsTx(ctx, tx, wsID, ids, names)
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

func (s *Store) DeleteDocument(ctx context.Context, wsID, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE id=$1 AND workspace_id=$2`, id, wsID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
