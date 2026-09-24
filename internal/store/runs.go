package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"raenil/internal/models"
)

// runCols is what every run query selects, in the order scanRun expects. The
// issue's key and title ride along so a list of runs reads without a lookup
// per row.
const runCols = `r.id, r.issue_id, coalesce(i.key, ''), coalesce(i.title, ''),
	r.agent_id, r.kind, r.runner, r.model, r.attempt, r.status, r.verdict, r.session_id, r.exit_code, r.agent_error,
	r.tokens_input, r.tokens_cache_read, r.tokens_cache_creation, r.tokens_output, r.tokens_total,
	r.cost_usd::float8, r.notional_cost_usd::float8, r.billing, r.denied_tools, r.log_tail, r.host,
	r.started_at, r.finished_at`

func scanRun(row pgx.Row) (models.Run, error) {
	var r models.Run
	var denied []byte
	err := row.Scan(&r.ID, &r.IssueID, &r.IssueKey, &r.IssueTitle,
		&r.AgentID, &r.Kind, &r.Runner, &r.Model, &r.Attempt, &r.Status, &r.Verdict, &r.SessionID, &r.ExitCode, &r.AgentError,
		&r.Tokens.Input, &r.Tokens.CacheRead, &r.Tokens.CacheCreation, &r.Tokens.Output, &r.Tokens.Total,
		&r.CostUSD, &r.NotionalUSD, &r.Billing, &denied, &r.LogTail, &r.Host,
		&r.StartedAt, &r.FinishedAt)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(denied, &r.DeniedTools); err != nil || r.DeniedTools == nil {
		r.DeniedTools = []string{}
	}
	return r, nil
}

// RunStart is what a machine knows when an attempt begins.
type RunStart struct {
	IssueID string
	AgentID string // optional
	Kind    string // work (default) | chat
	Runner  string
	Model   string
	Attempt int
	Host    string
}

// StartRun records that an attempt has begun on an issue in this workspace.
func (s *Store) StartRun(ctx context.Context, wsID string, in RunStart) (models.Run, error) {
	if in.Runner == "" {
		return models.Run{}, invalid("runner is required")
	}
	if in.Attempt < 1 {
		in.Attempt = 1
	}
	if in.Kind == "" {
		in.Kind = "work"
	}
	if in.Kind != "work" && in.Kind != "chat" {
		return models.Run{}, invalid("run kind must be work or chat, not %q", in.Kind)
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO runs (workspace_id, issue_id, runner, model, attempt, host, agent_id, kind)
		 SELECT $1, i.id, $3, $4, $5, $6,
		        (SELECT a.id FROM agents a WHERE a.id::text = $7 AND a.workspace_id = $1), $8
		 FROM issues i WHERE i.id = $2 AND i.workspace_id = $1
		 RETURNING id`,
		wsID, in.IssueID, in.Runner, in.Model, in.Attempt, in.Host, in.AgentID, in.Kind).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Run{}, ErrNotFound
	}
	if err != nil {
		return models.Run{}, err
	}
	return s.GetRun(ctx, wsID, id)
}

// RunFinish is what a machine reports when an attempt ends.
type RunFinish struct {
	Status      string
	Verdict     string
	SessionID   string
	ExitCode    *int
	AgentError  string
	Tokens      models.RunTokens
	CostUSD     float64
	NotionalUSD float64
	Billing     string
	DeniedTools []string
	LogTail     string
}

// maxLogTail caps what a run keeps of its transcript. The full log stays on
// the machine that ran it; this is enough to see how an attempt ended.
const maxLogTail = 64 * 1024

var runFinalStatus = map[string]bool{models.RunSucceeded: true, models.RunFailed: true, models.RunAborted: true}

var runBilling = map[string]bool{"subscription": true, "api": true, "unknown": true}

// FinishRun closes a running attempt. A run finishes once: a second report is
// refused rather than silently rewriting what was already recorded.
func (s *Store) FinishRun(ctx context.Context, wsID, runID string, f RunFinish) (models.Run, error) {
	if !runFinalStatus[f.Status] {
		return models.Run{}, invalid("status must be succeeded, failed or aborted, not %q", f.Status)
	}
	if f.Billing == "" {
		f.Billing = "unknown"
	}
	if !runBilling[f.Billing] {
		return models.Run{}, invalid("billing must be subscription, api or unknown, not %q", f.Billing)
	}
	if len(f.LogTail) > maxLogTail {
		f.LogTail = f.LogTail[len(f.LogTail)-maxLogTail:]
	}
	if f.DeniedTools == nil {
		f.DeniedTools = []string{}
	}
	denied, _ := json.Marshal(f.DeniedTools)
	if f.Tokens.Total == 0 {
		f.Tokens.Total = f.Tokens.Input + f.Tokens.CacheRead + f.Tokens.CacheCreation + f.Tokens.Output
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE runs SET status=$3, verdict=$4, session_id=$5, exit_code=$6, agent_error=$7,
		   tokens_input=$8, tokens_cache_read=$9, tokens_cache_creation=$10, tokens_output=$11, tokens_total=$12,
		   cost_usd=$13, notional_cost_usd=$14, billing=$15, denied_tools=$16, log_tail=$17, finished_at=now()
		 WHERE id=$2 AND workspace_id=$1 AND finished_at IS NULL`,
		wsID, runID, f.Status, f.Verdict, f.SessionID, f.ExitCode, f.AgentError,
		f.Tokens.Input, f.Tokens.CacheRead, f.Tokens.CacheCreation, f.Tokens.Output, f.Tokens.Total,
		f.CostUSD, f.NotionalUSD, f.Billing, denied, f.LogTail)
	if err != nil {
		return models.Run{}, err
	}
	if tag.RowsAffected() == 0 {
		existing, gerr := s.GetRun(ctx, wsID, runID)
		if gerr != nil {
			return models.Run{}, gerr
		}
		return existing, fmt.Errorf("run %s already finished as %s: %w", runID, existing.Status, ErrConflict)
	}
	return s.GetRun(ctx, wsID, runID)
}

// GetRun reads one run in this workspace.
func (s *Store) GetRun(ctx context.Context, wsID, runID string) (models.Run, error) {
	r, err := scanRun(s.pool.QueryRow(ctx,
		`SELECT `+runCols+` FROM runs r LEFT JOIN issues i ON i.id = r.issue_id
		 WHERE r.id = $2 AND r.workspace_id = $1`, wsID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// RunFilter narrows ListRuns. Zero values mean "any".
type RunFilter struct {
	IssueID string
	AgentID string
	Limit   int
	// WithLog includes each run's log tail. Lists leave it out: it is the one
	// field that can be tens of kilobytes.
	WithLog bool
}

// ListRuns lists runs newest first.
func (s *Store) ListRuns(ctx context.Context, wsID string, f RunFilter) ([]models.Run, error) {
	q := `SELECT ` + runCols + ` FROM runs r LEFT JOIN issues i ON i.id = r.issue_id WHERE r.workspace_id = $1`
	args := []any{wsID}
	if f.IssueID != "" {
		args = append(args, f.IssueID)
		q += fmt.Sprintf(" AND r.issue_id = $%d", len(args))
	}
	if f.AgentID != "" {
		args = append(args, f.AgentID)
		q += fmt.Sprintf(" AND r.agent_id = $%d", len(args))
	}
	q += " ORDER BY r.started_at DESC"
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	args = append(args, f.Limit)
	q += fmt.Sprintf(" LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		if !f.WithLog {
			r.LogTail = ""
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
