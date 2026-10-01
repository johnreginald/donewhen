package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- Users ----

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1,$2) RETURNING id, email, created_at`,
		email, passwordHash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	return u, err
}

// setupLockKey serialises first-run setup. Any int64 works; it only has to be
// the same for every caller.
const setupLockKey int64 = 0x72616e65696c // "raenil"

// CreateFirstUser creates the initial account, and only if no user exists yet.
// The check and the insert are one statement under a transaction-scoped
// advisory lock: NOT EXISTS alone is not enough, because two concurrent
// transactions can both see an empty table under READ COMMITTED. A second
// caller waits for the lock, then finds the first user and gets ErrConflict.
func (s *Store) CreateFirstUser(ctx context.Context, email, passwordHash string) (models.User, error) {
	var u models.User
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return u, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, setupLockKey); err != nil {
		return u, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM users)
		RETURNING id, email, created_at`, email, passwordHash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, fmt.Errorf("%w: already set up", ErrConflict)
	}
	if err != nil {
		return u, err
	}
	return u, tx.Commit(ctx)
}

func (s *Store) GetUser(ctx context.Context, id string) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, created_at FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// FirstUser returns the single account (single-user app convenience).
func (s *Store) FirstUser(ctx context.Context) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, created_at FROM users ORDER BY created_at LIMIT 1`).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// GetUserByEmail returns the user and their stored password hash.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (models.User, string, error) {
	var u models.User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, created_at, password_hash FROM users WHERE lower(email)=lower($1)`, email).
		Scan(&u.ID, &u.Email, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

// ---- Sessions ----

func (s *Store) CreateSession(ctx context.Context, id, userID string, expires time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1,$2,$3)`, id, userID, expires)
	return err
}

// LookupSession returns the user for a live (non-expired) session token.
func (s *Store) LookupSession(ctx context.Context, id string) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id=$1 AND s.expires_at > now()`, id).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

// ---- API tokens ----

type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	// WorkspaceID pins the token to one workspace. Nil means "any workspace the
	// owning user belongs to" — the caller then has to name one.
	WorkspaceID   *string `json:"workspaceId"`
	WorkspaceName *string `json:"workspaceName"`
}

// CreateAPIToken mints a token. Pass wsID to pin it to a single workspace, so
// an agent working in one repo can never see another workspace's issues.
func (s *Store) CreateAPIToken(ctx context.Context, userID, name, hash string, wsID *string) (APIToken, error) {
	var t APIToken
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, name, hash, workspace_id) VALUES ($1,$2,$3,$4)
		 RETURNING id, name, created_at, last_used_at, workspace_id`,
		userID, name, hash, wsID).Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.WorkspaceID)
	return t, err
}

func (s *Store) ListAPITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name, t.created_at, t.last_used_at, t.workspace_id, w.name
		FROM api_tokens t LEFT JOIN workspaces w ON w.id = t.workspace_id
		WHERE t.user_id=$1 ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.LastUsedAt, &t.WorkspaceID, &t.WorkspaceName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LookupAPIToken resolves a token hash to its owner and touches last_used_at.
// The second return is the workspace the token is pinned to, or nil when it may
// act on any workspace its owner belongs to.
func (s *Store) LookupAPIToken(ctx context.Context, hash string) (models.User, *string, error) {
	var u models.User
	var wsID *string
	err := s.pool.QueryRow(ctx, `
		UPDATE api_tokens t SET last_used_at=now()
		FROM users u
		WHERE t.hash=$1 AND u.id = t.user_id
		RETURNING u.id, u.email, u.created_at, t.workspace_id`,
		hash).Scan(&u.ID, &u.Email, &u.CreatedAt, &wsID)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, nil, ErrNotFound
	}
	return u, wsID, err
}

func (s *Store) DeleteAPIToken(ctx context.Context, userID, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM api_tokens WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Push subscriptions ----

func (s *Store) SavePushSubscription(ctx context.Context, userID string, sub models.PushSubscription) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (endpoint) DO UPDATE SET user_id=EXCLUDED.user_id, p256dh=EXCLUDED.p256dh, auth=EXCLUDED.auth`,
		userID, sub.Endpoint, sub.P256dh, sub.Auth)
	return err
}

// ListPushSubscriptions returns the devices that should be notified about a
// workspace: only those belonging to its members. Without the membership join a
// notification for one workspace would buzz a phone that cannot even open it.
func (s *Store) ListPushSubscriptions(ctx context.Context, wsID string) ([]models.PushSubscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.endpoint, p.p256dh, p.auth
		FROM push_subscriptions p
		JOIN workspace_members m ON m.user_id = p.user_id
		WHERE m.workspace_id = $1`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.PushSubscription
	for rows.Next() {
		var p models.PushSubscription
		if err := rows.Scan(&p.ID, &p.Endpoint, &p.P256dh, &p.Auth); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePushSubscription removes one of userID's own subscriptions. An endpoint
// that belongs to someone else (or does not exist) is ErrNotFound and nothing
// is deleted.
func (s *Store) DeletePushSubscription(ctx context.Context, userID, endpoint string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint=$1 AND user_id=$2`, endpoint, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePushSubscriptionByEndpoint removes a subscription whatever its owner.
// Only for the notifier, when the push service reports the endpoint gone; never
// call it on behalf of a user request.
func (s *Store) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint=$1`, endpoint)
	return err
}
