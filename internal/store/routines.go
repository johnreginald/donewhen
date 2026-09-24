package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"

	"raenil/internal/models"
)

const routineCols = `r.id, r.name, r.agent_id, r.project_id, r.title, r.description_md, r.criteria, r.schedule,
	r.timezone, r.enabled, r.auto_run, r.next_run_at, r.last_run_at, r.last_issue_id, coalesce(i.key, ''),
	r.created_at, r.updated_at`

func scanRoutine(row pgx.Row) (models.Routine, error) {
	var r models.Routine
	var crit []byte
	err := row.Scan(&r.ID, &r.Name, &r.AgentID, &r.ProjectID, &r.Title, &r.DescriptionMD, &crit, &r.Schedule,
		&r.Timezone, &r.Enabled, &r.AutoRun, &r.NextRunAt, &r.LastRunAt, &r.LastIssueID, &r.LastIssueKey,
		&r.CreatedAt, &r.UpdatedAt)
	r.Criteria = json.RawMessage(crit)
	return r, err
}

// NextRun is when a schedule next fires after a moment, in its timezone.
func NextRun(schedule, timezone string, after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(strings.TrimSpace(schedule))
	if err != nil {
		return time.Time{}, invalid("schedule %q is not a five-field cron: %v", schedule, err)
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, invalid("unknown timezone %q", timezone)
	}
	return sched.Next(after.In(loc)).UTC(), nil
}

// RoutineInput creates or changes a routine. Nil fields are left as they are.
type RoutineInput struct {
	Name          *string
	AgentID       *string
	SetAgent      bool
	ProjectID     *string
	SetProject    bool
	Title         *string
	DescriptionMD *string
	Criteria      []ProposedCriterion
	SetCriteria   bool
	Schedule      *string
	Timezone      *string
	Enabled       *bool
	AutoRun       *bool
}

// checkRoutine validates the routine as it will be after the change. A
// routine that runs its tickets must give them something a program can
// check, or the orchestrator would refuse every one.
func checkRoutine(r models.Routine) error {
	if strings.TrimSpace(r.Name) == "" {
		return invalid("name is required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return invalid("the ticket needs a title")
	}
	if _, err := NextRun(r.Schedule, r.Timezone, time.Now()); err != nil {
		return err
	}
	var crit []ProposedCriterion
	_ = json.Unmarshal(r.Criteria, &crit)
	if len(crit) > 0 {
		if _, err := NormaliseProposal(Proposal{Tickets: []ProposedTicket{{Title: r.Title, Criteria: crit}}}); err != nil && r.AutoRun {
			return err
		}
	} else if r.AutoRun {
		return invalid("a routine that runs its tickets needs a done-when a program can check")
	}
	if r.AutoRun && r.AgentID == nil {
		return invalid("a routine that runs its tickets needs an agent")
	}
	return nil
}

// CreateRoutine adds a routine and schedules its first run.
func (s *Store) CreateRoutine(ctx context.Context, wsID string, in RoutineInput) (models.Routine, error) {
	r := models.Routine{Timezone: "UTC", Enabled: true, Criteria: json.RawMessage(`[]`)}
	applyRoutine(&r, in)
	if err := checkRoutine(r); err != nil {
		return r, err
	}
	next, _ := NextRun(r.Schedule, r.Timezone, time.Now())
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO routines (workspace_id, name, agent_id, project_id, title, description_md, criteria, schedule,
		                      timezone, enabled, auto_run, next_run_at)
		VALUES ($1, $2,
		        (SELECT id FROM agents WHERE id::text = $3 AND workspace_id = $1),
		        (SELECT id FROM projects WHERE id::text = $4 AND workspace_id = $1),
		        $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		wsID, r.Name, deref(r.AgentID), deref(r.ProjectID), r.Title, r.DescriptionMD, []byte(r.Criteria),
		r.Schedule, r.Timezone, r.Enabled, r.AutoRun, next).Scan(&id)
	if err != nil {
		return r, err
	}
	return s.GetRoutine(ctx, wsID, id)
}

// UpdateRoutine changes a routine, rescheduling it from now.
func (s *Store) UpdateRoutine(ctx context.Context, wsID, id string, in RoutineInput) (models.Routine, error) {
	r, err := s.GetRoutine(ctx, wsID, id)
	if err != nil {
		return r, err
	}
	applyRoutine(&r, in)
	if err := checkRoutine(r); err != nil {
		return r, err
	}
	next, _ := NextRun(r.Schedule, r.Timezone, time.Now())
	_, err = s.pool.Exec(ctx, `
		UPDATE routines SET name = $3,
		       agent_id = (SELECT id FROM agents WHERE id::text = $4 AND workspace_id = $1),
		       project_id = (SELECT id FROM projects WHERE id::text = $5 AND workspace_id = $1),
		       title = $6, description_md = $7, criteria = $8, schedule = $9, timezone = $10,
		       enabled = $11, auto_run = $12, next_run_at = $13, updated_at = now()
		WHERE workspace_id = $1 AND id::text = $2`,
		wsID, r.ID, r.Name, deref(r.AgentID), deref(r.ProjectID), r.Title, r.DescriptionMD, []byte(r.Criteria),
		r.Schedule, r.Timezone, r.Enabled, r.AutoRun, next)
	if err != nil {
		return r, err
	}
	return s.GetRoutine(ctx, wsID, r.ID)
}

func applyRoutine(r *models.Routine, in RoutineInput) {
	if in.Name != nil {
		r.Name = strings.TrimSpace(*in.Name)
	}
	if in.SetAgent {
		r.AgentID = in.AgentID
		if r.AgentID != nil && *r.AgentID == "" {
			r.AgentID = nil
		}
	}
	if in.SetProject {
		r.ProjectID = in.ProjectID
		if r.ProjectID != nil && *r.ProjectID == "" {
			r.ProjectID = nil
		}
	}
	if in.Title != nil {
		r.Title = strings.TrimSpace(*in.Title)
	}
	if in.DescriptionMD != nil {
		r.DescriptionMD = *in.DescriptionMD
	}
	if in.SetCriteria {
		b, _ := json.Marshal(in.Criteria)
		if in.Criteria == nil {
			b = []byte(`[]`)
		}
		r.Criteria = b
	}
	if in.Schedule != nil {
		r.Schedule = strings.TrimSpace(*in.Schedule)
	}
	if in.Timezone != nil && strings.TrimSpace(*in.Timezone) != "" {
		r.Timezone = strings.TrimSpace(*in.Timezone)
	}
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	}
	if in.AutoRun != nil {
		r.AutoRun = *in.AutoRun
	}
}

// GetRoutine reads one routine.
func (s *Store) GetRoutine(ctx context.Context, wsID, id string) (models.Routine, error) {
	r, err := scanRoutine(s.pool.QueryRow(ctx, `SELECT `+routineCols+`
		FROM routines r LEFT JOIN issues i ON i.id = r.last_issue_id
		WHERE r.workspace_id = $1 AND r.id::text = $2`, wsID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// ListRoutines lists a workspace's routines by name.
func (s *Store) ListRoutines(ctx context.Context, wsID string) ([]models.Routine, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+routineCols+`
		FROM routines r LEFT JOIN issues i ON i.id = r.last_issue_id
		WHERE r.workspace_id = $1 ORDER BY r.name`, wsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Routine
	for rows.Next() {
		r, err := scanRoutine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRoutine removes a routine; the tickets it made stay.
func (s *Store) DeleteRoutine(ctx context.Context, wsID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM routines WHERE workspace_id = $1 AND id::text = $2`, wsID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DueRoutine is a routine whose time has come, with its workspace.
type DueRoutine struct {
	WorkspaceID string
	models.Routine
}

// ClaimDueRoutines takes the routines due by now and moves each one's next
// run on. SKIP LOCKED plus the move means two servers never fire one twice.
func (s *Store) ClaimDueRoutines(ctx context.Context, now time.Time) ([]DueRoutine, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT r.workspace_id::text, `+routineCols+`
		FROM routines r LEFT JOIN issues i ON i.id = r.last_issue_id
		WHERE r.enabled AND r.next_run_at <= $1
		ORDER BY r.next_run_at
		FOR UPDATE OF r SKIP LOCKED
		LIMIT 50`, now)
	if err != nil {
		return nil, err
	}
	var due []DueRoutine
	for rows.Next() {
		var d DueRoutine
		var crit []byte
		err := rows.Scan(&d.WorkspaceID, &d.ID, &d.Name, &d.AgentID, &d.ProjectID, &d.Title, &d.DescriptionMD, &crit,
			&d.Schedule, &d.Timezone, &d.Enabled, &d.AutoRun, &d.NextRunAt, &d.LastRunAt, &d.LastIssueID,
			&d.LastIssueKey, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			rows.Close()
			return nil, err
		}
		d.Criteria = crit
		due = append(due, d)
	}
	rows.Close()
	for _, d := range due {
		// A schedule that stopped parsing is switched off rather than left
		// firing on every tick.
		next, err := NextRun(d.Schedule, d.Timezone, now)
		if err != nil {
			if _, err := tx.Exec(ctx, `UPDATE routines SET enabled = false WHERE id = $1`, d.ID); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE routines SET next_run_at = $2, last_run_at = $3 WHERE id = $1`,
			d.ID, next, now); err != nil {
			return nil, err
		}
	}
	return due, tx.Commit(ctx)
}

// RoutineFired records the ticket a routine made.
func (s *Store) RoutineFired(ctx context.Context, wsID, routineID, issueID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE routines SET last_issue_id = $3, last_run_at = now()
		WHERE workspace_id = $1 AND id::text = $2`, wsID, routineID, issueID)
	return err
}

// RoutineStillOpen reports whether the ticket a routine last made is still
// open — Paperclip's "coalesce if active": one open routine ticket at a time.
func (s *Store) RoutineStillOpen(ctx context.Context, r models.Routine) (bool, error) {
	if r.LastIssueID == nil {
		return false, nil
	}
	var open bool
	err := s.pool.QueryRow(ctx, `
		SELECT ws.category NOT IN ('completed', 'canceled')
		FROM issues i JOIN workflow_states ws ON ws.id = i.state_id WHERE i.id = $1`, *r.LastIssueID).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return open, err
}

// ---- agent heartbeats ----

// DueHeartbeat is an agent whose timer came round, with its workspace.
type DueHeartbeat struct {
	WorkspaceID string
	AgentID     string
}

// ClaimDueHeartbeats takes the active agents whose heartbeat is due.
func (s *Store) ClaimDueHeartbeats(ctx context.Context, now time.Time) ([]DueHeartbeat, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE agents SET last_heartbeat_at = $1
		WHERE id IN (
			SELECT id FROM agents
			WHERE status = 'active' AND heartbeat_minutes > 0
			  AND (last_heartbeat_at IS NULL OR last_heartbeat_at + make_interval(mins => heartbeat_minutes) <= $1)
			FOR UPDATE SKIP LOCKED
		)
		RETURNING workspace_id::text, id::text`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueHeartbeat
	for rows.Next() {
		var d DueHeartbeat
		if err := rows.Scan(&d.WorkspaceID, &d.AgentID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// WaitingForAgent lists the agent's open tickets where a person has written
// since the agent last replied, and no turn is already queued or running.
func (s *Store) WaitingForAgent(ctx context.Context, wsID, agentID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text FROM issues i JOIN workflow_states ws ON ws.id = i.state_id
		WHERE i.workspace_id = $1 AND i.agent_id::text = $2
		  AND ws.category NOT IN ('completed', 'canceled')
		  AND EXISTS (
			SELECT 1 FROM comments c WHERE c.issue_id = i.id AND c.agent_id IS NULL AND c.actor = 'human'
			  AND c.created_at > coalesce((SELECT max(a.created_at) FROM comments a
			                               WHERE a.issue_id = i.id AND a.agent_id::text = $2), '-infinity'))
		  AND NOT EXISTS (
			SELECT 1 FROM jobs j WHERE j.issue_id = i.id AND j.kind = 'chat' AND j.status IN ('queued', 'claimed'))`,
		wsID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RoutineTitle fills {date} with the day the routine fired, in its timezone.
func RoutineTitle(r models.Routine, at time.Time) string {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return strings.ReplaceAll(r.Title, "{date}", at.In(loc).Format("2006-01-02"))
}
