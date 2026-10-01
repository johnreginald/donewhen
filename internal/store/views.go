package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// View is a saved filter: the URL query of the issue list or board it was
// saved from, private to its owner within one workspace.
type View struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`  // the URL query string, without the leading "?"
	Layout    string    `json:"layout"` // board | list
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

// ViewPatch changes some fields of a view; nil leaves a field alone.
type ViewPatch struct {
	Name     *string
	Query    *string
	Layout   *string
	Position *int
}

const (
	maxViewName  = 60
	maxViewQuery = 2000
)

// defaultViews are created for each user the first time they list views.
var defaultViews = []struct{ name, query string }{
	{"In Review", "state=In+Review"},
	{"Blocked", "state=Blocked"},
	{"Ready queue", "state=Ready"},
}

func cleanView(name, query, layout string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxViewName {
		return "", "", "", invalid("view name must be 1-%d characters", maxViewName)
	}
	query = strings.TrimPrefix(strings.TrimSpace(query), "?")
	if len(query) > maxViewQuery {
		return "", "", "", invalid("view query is too long")
	}
	if layout != "board" && layout != "list" {
		return "", "", "", invalid("view layout must be board or list")
	}
	return name, query, layout, nil
}

// ListViews returns the user's views in this workspace in sidebar order,
// creating the three defaults the first time.
func (s *Store) ListViews(ctx context.Context, wsID, userID string) ([]View, error) {
	if err := s.seedViews(ctx, wsID, userID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, query, layout, position, created_at FROM views
		WHERE workspace_id=$1 AND user_id=$2 ORDER BY position, created_at`, wsID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []View{}
	for rows.Next() {
		var v View
		if err := rows.Scan(&v.ID, &v.Name, &v.Query, &v.Layout, &v.Position, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// seedViews inserts the default views once per user and workspace.
func (s *Store) seedViews(ctx context.Context, wsID, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ct, err := tx.Exec(ctx,
		`INSERT INTO views_seeded (workspace_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, wsID, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return nil
	}
	for i, d := range defaultViews {
		if _, err := tx.Exec(ctx,
			`INSERT INTO views (workspace_id, user_id, name, query, layout, position) VALUES ($1,$2,$3,$4,'list',$5)`,
			wsID, userID, d.name, d.query, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CreateView adds a view at the end of the user's list.
func (s *Store) CreateView(ctx context.Context, wsID, userID, name, query, layout string) (View, error) {
	name, query, layout, err := cleanView(name, query, layout)
	if err != nil {
		return View{}, err
	}
	var v View
	err = s.pool.QueryRow(ctx, `
		INSERT INTO views (workspace_id, user_id, name, query, layout, position)
		VALUES ($1,$2,$3,$4,$5, coalesce((SELECT max(position)+1 FROM views WHERE workspace_id=$1 AND user_id=$2), 0))
		RETURNING id, name, query, layout, position, created_at`,
		wsID, userID, name, query, layout).Scan(&v.ID, &v.Name, &v.Query, &v.Layout, &v.Position, &v.CreatedAt)
	return v, err
}

// UpdateView changes a view its owner holds; anyone else gets ErrNotFound.
func (s *Store) UpdateView(ctx context.Context, wsID, userID, id string, p ViewPatch) (View, error) {
	cur, err := s.getView(ctx, wsID, userID, id)
	if err != nil {
		return View{}, err
	}
	if p.Name != nil {
		cur.Name = *p.Name
	}
	if p.Query != nil {
		cur.Query = *p.Query
	}
	if p.Layout != nil {
		cur.Layout = *p.Layout
	}
	if p.Position != nil {
		cur.Position = *p.Position
	}
	cur.Name, cur.Query, cur.Layout, err = cleanView(cur.Name, cur.Query, cur.Layout)
	if err != nil {
		return View{}, err
	}
	err = s.pool.QueryRow(ctx, `
		UPDATE views SET name=$4, query=$5, layout=$6, position=$7
		WHERE id=$1 AND workspace_id=$2 AND user_id=$3
		RETURNING id, name, query, layout, position, created_at`,
		id, wsID, userID, cur.Name, cur.Query, cur.Layout, cur.Position,
	).Scan(&cur.ID, &cur.Name, &cur.Query, &cur.Layout, &cur.Position, &cur.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	return cur, err
}

func (s *Store) getView(ctx context.Context, wsID, userID, id string) (View, error) {
	var v View
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, query, layout, position, created_at FROM views
		WHERE id::text=$1 AND workspace_id=$2 AND user_id=$3`, id, wsID, userID,
	).Scan(&v.ID, &v.Name, &v.Query, &v.Layout, &v.Position, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, ErrNotFound
	}
	return v, err
}

// DeleteView removes a view its owner holds.
func (s *Store) DeleteView(ctx context.Context, wsID, userID, id string) error {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM views WHERE id::text=$1 AND workspace_id=$2 AND user_id=$3`, id, wsID, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
