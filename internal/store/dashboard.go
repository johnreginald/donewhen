package store

import (
	"context"
	"time"

	"raenil/internal/models"
)

// Dashboard is the workspace at a glance, as Paperclip's dashboard shows it:
// who is working on what, four headline numbers, fourteen days of activity,
// and what happened last.
type Dashboard struct {
	Agents   []DashboardAgent  `json:"agents"`
	KPIs     DashboardKPIs     `json:"kpis"`
	Days     []DashboardDay    `json:"days"`
	Activity []models.Activity `json:"activity"`
	Recent   []DashboardTask   `json:"recentTasks"`
}

// DashboardAgent is one agent and the last thing it did.
type DashboardAgent struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	AgentStatus string     `json:"agentStatus"` // active | paused
	Runner      string     `json:"runner"`
	Model       string     `json:"model"`
	Status      string     `json:"status"` // the latest run's; "" if it never ran
	IssueKey    string     `json:"issueKey"`
	IssueTitle  string     `json:"issueTitle"`
	StartedAt   *time.Time `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
}

// DashboardKPIs are the four headline cards.
type DashboardKPIs struct {
	AgentsActive    int     `json:"agentsActive"` // agents not paused
	AgentsPaused    int     `json:"agentsPaused"`
	RunsRunning     int     `json:"runsRunning"`      // right now
	InProgress      int     `json:"inProgress"`       // tickets in a started state
	Open            int     `json:"open"`             // tickets not done or canceled
	Blocked         int     `json:"blocked"`          // tickets in Blocked
	InReview        int     `json:"inReview"`         // tickets waiting on a human
	MonthCostUSD    float64 `json:"monthCostUsd"`     // metered spend this month
	MonthNotionalUS float64 `json:"monthNotionalUsd"` // subscription usage at list price
	MonthTokens     int64   `json:"monthTokens"`
}

// DashboardDay is one day of the three charts.
type DashboardDay struct {
	Date string `json:"date"` // YYYY-MM-DD in the caller's timezone
	// Runs started that day, by how the agent's attempt ended.
	RunsSucceeded int `json:"runsSucceeded"`
	RunsFailed    int `json:"runsFailed"`
	RunsOther     int `json:"runsOther"` // running, queued, aborted
	// Runs whose criteria passed, of runs that finished that day.
	Passed   int `json:"passed"`
	Finished int `json:"finished"`
	// Tickets moved into each kind of state that day.
	MovedReady   int `json:"movedReady"`
	MovedStarted int `json:"movedStarted"`
	MovedDone    int `json:"movedDone"`
}

// DashboardTask is a recently touched ticket.
type DashboardTask struct {
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	StateID   string    `json:"stateId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// dashboardDays is how far back the charts reach, today included.
const dashboardDays = 14

// Dashboard assembles the dashboard for one workspace. tz names the timezone
// days are counted in, so "today" is the viewer's today, not the server's.
func (s *Store) Dashboard(ctx context.Context, wsID string, tz *time.Location) (Dashboard, error) {
	var d Dashboard
	zone := tz.String()

	// Agents, each with its most recent run.
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.name, a.slug, a.status, a.harness, coalesce(lr.model, a.model),
		       coalesce(lr.status, ''), coalesce(lr.key, ''), coalesce(lr.title, ''), lr.started_at, lr.finished_at
		FROM agents a
		LEFT JOIN LATERAL (
			SELECT r.model, r.status, i.key, i.title, r.started_at, r.finished_at
			FROM runs r LEFT JOIN issues i ON i.id = r.issue_id
			WHERE r.agent_id = a.id ORDER BY r.started_at DESC LIMIT 1
		) lr ON true
		WHERE a.workspace_id = $1
		ORDER BY lr.started_at DESC NULLS LAST, a.name`, wsID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var a DashboardAgent
		if err := rows.Scan(&a.ID, &a.Name, &a.Slug, &a.AgentStatus, &a.Runner, &a.Model, &a.Status, &a.IssueKey,
			&a.IssueTitle, &a.StartedAt, &a.FinishedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Agents = append(d.Agents, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}

	// KPIs.
	k := &d.KPIs
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE ws.category = 'started'),
		       count(*) FILTER (WHERE ws.category NOT IN ('completed', 'canceled')),
		       count(*) FILTER (WHERE lower(ws.name) = 'blocked'),
		       count(*) FILTER (WHERE lower(ws.name) = 'in review')
		FROM issues i JOIN workflow_states ws ON ws.id = i.state_id
		WHERE i.workspace_id = $1`, wsID).Scan(&k.InProgress, &k.Open, &k.Blocked, &k.InReview)
	if err != nil {
		return d, err
	}
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status IN ('running', 'queued')),
		       coalesce(sum(cost_usd) FILTER (WHERE started_at >= date_trunc('month', now() AT TIME ZONE $2) AT TIME ZONE $2), 0)::float8,
		       coalesce(sum(notional_cost_usd) FILTER (WHERE started_at >= date_trunc('month', now() AT TIME ZONE $2) AT TIME ZONE $2), 0)::float8,
		       coalesce(sum(tokens_total) FILTER (WHERE started_at >= date_trunc('month', now() AT TIME ZONE $2) AT TIME ZONE $2), 0)
		FROM runs WHERE workspace_id = $1`, wsID, zone).
		Scan(&k.RunsRunning, &k.MonthCostUSD, &k.MonthNotionalUS, &k.MonthTokens)
	if err != nil {
		return d, err
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status = 'active'), count(*) FILTER (WHERE status = 'paused')
		FROM agents WHERE workspace_id = $1`, wsID).Scan(&k.AgentsActive, &k.AgentsPaused)
	if err != nil {
		return d, err
	}

	// Fourteen days, every day present even when nothing happened.
	rows, err = s.pool.Query(ctx, `
		WITH days AS (
			SELECT generate_series((now() AT TIME ZONE $2)::date - ($3::int - 1), (now() AT TIME ZONE $2)::date, interval '1 day')::date AS day
		),
		started AS (
			SELECT (started_at AT TIME ZONE $2)::date AS day,
			       count(*) FILTER (WHERE status = 'succeeded') AS ok,
			       count(*) FILTER (WHERE status = 'failed') AS bad,
			       count(*) FILTER (WHERE status NOT IN ('succeeded', 'failed')) AS other
			FROM runs WHERE workspace_id = $1 GROUP BY 1
		),
		finished AS (
			SELECT (finished_at AT TIME ZONE $2)::date AS day,
			       count(*) FILTER (WHERE verdict = 'passed') AS passed,
			       count(*) AS total
			FROM runs WHERE workspace_id = $1 AND finished_at IS NOT NULL GROUP BY 1
		),
		moved AS (
			SELECT (a.created_at AT TIME ZONE $2)::date AS day,
			       count(*) FILTER (WHERE ws.category = 'unstarted') AS ready,
			       count(*) FILTER (WHERE ws.category = 'started') AS started,
			       count(*) FILTER (WHERE ws.category = 'completed') AS done
			FROM activity a
			JOIN workflow_states ws ON ws.workspace_id = a.workspace_id AND ws.name = a.to_val
			WHERE a.workspace_id = $1 AND a.kind = 'state_changed'
			GROUP BY 1
		)
		SELECT to_char(days.day, 'YYYY-MM-DD'),
		       coalesce(s.ok, 0), coalesce(s.bad, 0), coalesce(s.other, 0),
		       coalesce(f.passed, 0), coalesce(f.total, 0),
		       coalesce(m.ready, 0), coalesce(m.started, 0), coalesce(m.done, 0)
		FROM days
		LEFT JOIN started s ON s.day = days.day
		LEFT JOIN finished f ON f.day = days.day
		LEFT JOIN moved m ON m.day = days.day
		ORDER BY days.day`, wsID, zone, dashboardDays)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var day DashboardDay
		if err := rows.Scan(&day.Date, &day.RunsSucceeded, &day.RunsFailed, &day.RunsOther,
			&day.Passed, &day.Finished, &day.MovedReady, &day.MovedStarted, &day.MovedDone); err != nil {
			rows.Close()
			return d, err
		}
		d.Days = append(d.Days, day)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}

	d.Activity, err = s.ListRecentActivity(ctx, ActivityFilter{WorkspaceID: wsID, Limit: 10})
	if err != nil {
		return d, err
	}

	rows, err = s.pool.Query(ctx, `
		SELECT key, title, state_id, updated_at FROM issues
		WHERE workspace_id = $1 ORDER BY updated_at DESC LIMIT 6`, wsID)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var t DashboardTask
		if err := rows.Scan(&t.Key, &t.Title, &t.StateID, &t.UpdatedAt); err != nil {
			return d, err
		}
		d.Recent = append(d.Recent, t)
	}
	if d.Agents == nil {
		d.Agents = []DashboardAgent{}
	}
	if d.Activity == nil {
		d.Activity = []models.Activity{}
	}
	if d.Recent == nil {
		d.Recent = []DashboardTask{}
	}
	return d, rows.Err()
}
