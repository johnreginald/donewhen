package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// ---- runner hosts ----

// HostHeartbeat records that a machine is alive and what it can run.
func (s *Store) HostHeartbeat(ctx context.Context, wsID, name, version string, harnesses []models.HarnessStatus) (models.RunnerHost, error) {
	if strings.TrimSpace(name) == "" {
		return models.RunnerHost{}, invalid("host name is required")
	}
	if harnesses == nil {
		harnesses = []models.HarnessStatus{}
	}
	b, _ := json.Marshal(harnesses)
	var h models.RunnerHost
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		INSERT INTO runner_hosts (workspace_id, name, version, harnesses, last_seen_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (workspace_id, name) DO UPDATE
		   SET version = EXCLUDED.version, harnesses = EXCLUDED.harnesses, last_seen_at = now()
		RETURNING id, name, harnesses, version, last_seen_at`,
		wsID, name, version, b).Scan(&h.ID, &h.Name, &raw, &h.Version, &h.LastSeenAt)
	if err != nil {
		return h, err
	}
	_ = json.Unmarshal(raw, &h.Harnesses)
	return h, nil
}

// ListHosts lists the machines that have reported in, most recent first.
func (s *Store) ListHosts(ctx context.Context, wsID string) ([]models.RunnerHost, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, harnesses, version, last_seen_at FROM runner_hosts
		WHERE workspace_id = $1 ORDER BY last_seen_at DESC`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.RunnerHost
	for rows.Next() {
		var h models.RunnerHost
		var raw []byte
		if err := rows.Scan(&h.ID, &h.Name, &raw, &h.Version, &h.LastSeenAt); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &h.Harnesses) != nil || h.Harnesses == nil {
			h.Harnesses = []models.HarnessStatus{}
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ---- jobs ----

const jobCols = `j.id, j.kind, j.agent_id, j.input, j.issue_id, coalesce(i.key, ''), j.status, j.host, j.result, j.error,
	j.created_at, j.claimed_at, j.finished_at`

func scanJob(row pgx.Row) (models.Job, error) {
	var j models.Job
	var result, input []byte
	err := row.Scan(&j.ID, &j.Kind, &j.AgentID, &input, &j.IssueID, &j.IssueKey, &j.Status, &j.Host, &result, &j.Error,
		&j.CreatedAt, &j.ClaimedAt, &j.FinishedAt)
	j.Result, j.Input = json.RawMessage(result), json.RawMessage(input)
	return j, err
}

// JobInput queues work for a runner host.
type JobInput struct {
	Kind    string
	AgentID string
	IssueID string
	Input   any // job-specific: what a chat turn should say, for one
}

// EnqueueJob queues a job. The agent and issue must be in this workspace.
func (s *Store) EnqueueJob(ctx context.Context, wsID string, in JobInput) (models.Job, error) {
	switch in.Kind {
	case "test_env":
		if in.AgentID == "" {
			return models.Job{}, invalid("a test needs an agent")
		}
	case "run_ticket", "chat":
		if in.AgentID == "" || in.IssueID == "" {
			return models.Job{}, invalid("a %s job needs an agent and a ticket", in.Kind)
		}
	default:
		return models.Job{}, invalid("unknown job kind %q", in.Kind)
	}
	if in.Kind == "chat" {
		// A turn that has not started yet will read every message waiting for
		// it, so a second message joins that turn instead of queueing another.
		var id string
		err := s.pool.QueryRow(ctx, `
			SELECT id FROM jobs WHERE workspace_id = $1 AND issue_id::text = $2 AND agent_id::text = $3
			  AND kind = 'chat' AND status = 'queued' ORDER BY created_at LIMIT 1`,
			wsID, in.IssueID, in.AgentID).Scan(&id)
		if err == nil {
			return s.GetJob(ctx, wsID, id)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return models.Job{}, err
		}
	}
	if in.Kind == "run_ticket" {
		// One run of a ticket at a time: two would cut two worktrees from the
		// same ticket and race each other to commit.
		var busy bool
		if err := s.pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM jobs WHERE workspace_id = $1 AND issue_id::text = $2
			              AND kind = 'run_ticket' AND status IN ('queued', 'claimed'))`,
			wsID, in.IssueID).Scan(&busy); err != nil {
			return models.Job{}, err
		}
		if busy {
			return models.Job{}, fmt.Errorf("this ticket is already queued or running: %w", ErrConflict)
		}
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO jobs (workspace_id, kind, agent_id, issue_id, input)
		SELECT $1, $2, a.id, i.id, $5
		FROM agents a
		LEFT JOIN issues i ON i.id::text = $4 AND i.workspace_id = $1
		WHERE a.id::text = $3 AND a.workspace_id = $1 AND ($4 = '' OR i.id IS NOT NULL)
		RETURNING id`, wsID, in.Kind, in.AgentID, in.IssueID, jobInput(in.Input)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Job{}, ErrNotFound
	}
	if err != nil {
		return models.Job{}, err
	}
	return s.GetJob(ctx, wsID, id)
}

// ClaimJob hands the oldest queued job this host can run to it. SKIP LOCKED
// means two hosts polling at once never take the same job.
func (s *Store) ClaimJob(ctx context.Context, wsID, host string, harnesses []string) (models.Job, bool, error) {
	if len(harnesses) == 0 {
		return models.Job{}, false, nil
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		UPDATE jobs SET status = 'claimed', host = $2, claimed_at = now()
		WHERE id = (
			SELECT j.id FROM jobs j JOIN agents a ON a.id = j.agent_id
			WHERE j.workspace_id = $1 AND j.status = 'queued' AND a.harness = ANY($3) AND a.status = 'active'
			ORDER BY j.created_at
			FOR UPDATE OF j SKIP LOCKED
			LIMIT 1
		)
		RETURNING id`, wsID, host, harnesses).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Job{}, false, nil
	}
	if err != nil {
		return models.Job{}, false, err
	}
	j, err := s.GetJob(ctx, wsID, id)
	return j, err == nil, err
}

// FinishJob records how a claimed job ended. Only the host that claimed it
// can finish it, and only once.
func (s *Store) FinishJob(ctx context.Context, wsID, id, host, status string, result json.RawMessage, errText string) (models.Job, error) {
	if status != "succeeded" && status != "failed" {
		return models.Job{}, invalid("status must be succeeded or failed, not %q", status)
	}
	if len(result) == 0 {
		result = json.RawMessage(`{}`)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status = $4, result = $5, error = $6, finished_at = now()
		WHERE workspace_id = $1 AND id = $2 AND host = $3 AND status = 'claimed'`,
		wsID, id, host, status, []byte(result), errText)
	if err != nil {
		return models.Job{}, err
	}
	if tag.RowsAffected() == 0 {
		return models.Job{}, fmt.Errorf("job %s is not claimed by %s: %w", id, host, ErrConflict)
	}
	return s.GetJob(ctx, wsID, id)
}

// GetJob reads one job.
func (s *Store) GetJob(ctx context.Context, wsID, id string) (models.Job, error) {
	j, err := scanJob(s.pool.QueryRow(ctx,
		`SELECT `+jobCols+` FROM jobs j LEFT JOIN issues i ON i.id = j.issue_id
		 WHERE j.workspace_id = $1 AND j.id::text = $2`, wsID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// JobFilter narrows ListJobs.
type JobFilter struct {
	AgentID string
	IssueID string
	Kind    string
	Limit   int
}

// ListJobs lists jobs newest first.
func (s *Store) ListJobs(ctx context.Context, wsID string, f JobFilter) ([]models.Job, error) {
	q := `SELECT ` + jobCols + ` FROM jobs j LEFT JOIN issues i ON i.id = j.issue_id WHERE j.workspace_id = $1`
	args := []any{wsID}
	add := func(cond string, v string) {
		args = append(args, v)
		q += fmt.Sprintf(" AND %s = $%d", cond, len(args))
	}
	if f.AgentID != "" {
		add("j.agent_id::text", f.AgentID)
	}
	if f.IssueID != "" {
		add("j.issue_id::text", f.IssueID)
	}
	if f.Kind != "" {
		add("j.kind", f.Kind)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	args = append(args, f.Limit)
	q += fmt.Sprintf(" ORDER BY j.created_at DESC LIMIT $%d", len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func jobInput(v any) []byte {
	if v == nil {
		return []byte(`{}`)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

// ---- reaping: work whose machine went away ----

// Reaped is what one workspace had closed for it.
type Reaped struct {
	Jobs []models.Job
	Runs []models.Run
}

// ReleaseHost fails every job a host had claimed and aborts every run it had
// open — what a host does on its first heartbeat after a restart.
func (s *Store) ReleaseHost(ctx context.Context, wsID, host, why string) ([]models.Job, []models.Run, error) {
	jobs, err := s.failJobs(ctx, `WHERE workspace_id = $1 AND host = $2 AND status = 'claimed'`, why, wsID, host)
	if err != nil {
		return nil, nil, err
	}
	runs, err := s.abortRuns(ctx, `WHERE workspace_id = $1 AND host = $2 AND status = 'running'`, why, wsID, host)
	return jobs, runs, err
}

// ReapStale fails jobs claimed by hosts silent for longer than after, and
// aborts runs still open on such hosts or older than maxRun, in every
// workspace.
func (s *Store) ReapStale(ctx context.Context, after, maxRun time.Duration) (map[string]Reaped, error) {
	out := map[string]Reaped{}
	gone := `(SELECT 1 FROM runner_hosts h WHERE h.workspace_id = x.workspace_id AND h.name = x.host
	           AND h.last_seen_at < now() - $1::interval)`
	jobs, err := s.failJobs(ctx, `x WHERE x.status = 'claimed' AND (EXISTS `+gone+`
		OR (x.claimed_at < now() - $1::interval AND NOT EXISTS (SELECT 1 FROM runner_hosts h
		    WHERE h.workspace_id = x.workspace_id AND h.name = x.host)))`,
		"its host stopped reporting", after.String())
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		r := out[j.WorkspaceID]
		r.Jobs = append(r.Jobs, j)
		out[j.WorkspaceID] = r
	}
	runs, err := s.abortRuns(ctx, `x WHERE x.status = 'running' AND (EXISTS `+gone+`
		OR x.started_at < now() - $2::interval)`,
		"never finished: its host stopped reporting", after.String(), maxRun.String())
	if err != nil {
		return nil, err
	}
	for _, rn := range runs {
		r := out[rn.WorkspaceID]
		r.Runs = append(r.Runs, rn)
		out[rn.WorkspaceID] = r
	}
	return out, nil
}

// failJobs marks matching claimed jobs failed. where may alias jobs as x; its
// placeholders start at $1, and why is appended after them.
func (s *Store) failJobs(ctx context.Context, where, why string, args ...any) ([]models.Job, error) {
	if !strings.HasPrefix(strings.TrimSpace(where), "x ") {
		where = "x " + where
	}
	args = append(args, why)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		UPDATE jobs AS %s
		RETURNING x.id::text, x.workspace_id::text`, strings.Replace(where, "WHERE",
		fmt.Sprintf("SET status = 'failed', error = $%d, finished_at = now() WHERE", len(args)), 1)), args...)
	if err != nil {
		return nil, err
	}
	type key struct{ id, ws string }
	var keys []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.id, &k.ws); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	var out []models.Job
	for _, k := range keys {
		j, err := s.GetJob(ctx, k.ws, k.id)
		if err != nil {
			return nil, err
		}
		j.WorkspaceID = k.ws
		out = append(out, j)
	}
	return out, rows.Err()
}

// abortRuns marks matching open runs aborted, the same way failJobs works.
func (s *Store) abortRuns(ctx context.Context, where, why string, args ...any) ([]models.Run, error) {
	if !strings.HasPrefix(strings.TrimSpace(where), "x ") {
		where = "x " + where
	}
	args = append(args, why)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		UPDATE runs AS %s
		RETURNING x.id::text, x.workspace_id::text`, strings.Replace(where, "WHERE",
		fmt.Sprintf("SET status = 'aborted', agent_error = $%d, finished_at = now() WHERE", len(args)), 1)), args...)
	if err != nil {
		return nil, err
	}
	type key struct{ id, ws string }
	var keys []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.id, &k.ws); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	var out []models.Run
	for _, k := range keys {
		r, err := s.GetRun(ctx, k.ws, k.id)
		if err != nil {
			return nil, err
		}
		r.WorkspaceID = k.ws
		out = append(out, r)
	}
	return out, rows.Err()
}
