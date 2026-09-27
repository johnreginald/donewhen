package store

import (
	"context"
	"time"
)

// Flow is how well the factory is moving over a window of days: per
// harness, per ticket, the reviews, and what waits on the user.
type Flow struct {
	Days    int          `json:"days"`
	Runners []RunnerFlow `json:"runners"`
	// Tickets that passed in the window, and the attempts and hours they took.
	TicketsPassed      int     `json:"ticketsPassed"`
	AttemptsPerPassed  float64 `json:"attemptsPerPassed"`
	MedianTicketHours  float64 `json:"medianTicketHours"`
	CodeReviews        int     `json:"codeReviews"`
	CodeFirstPassRate  float64 `json:"codeFirstPassRate"` // first-round reviews with nothing blocking
	BlockingFound      int     `json:"blockingFound"`
	TicketReviews      int     `json:"ticketReviews"`
	TicketQualifyRate  float64 `json:"ticketQualifyRate"`
	WaitingForReview   int     `json:"waitingForReview"` // In Review now
	WaitingHoursMedian float64 `json:"waitingHoursMedian"`
}

// RunnerFlow is one harness's work runs in the window.
type RunnerFlow struct {
	Runner        string  `json:"runner"`
	Runs          int     `json:"runs"`
	Passed        int     `json:"passed"`
	PassRate      float64 `json:"passRate"`
	MedianMinutes float64 `json:"medianMinutes"`
	Tokens        int64   `json:"tokens"`
	CostUSD       float64 `json:"costUsd"`
}

// FlowStats reads the factory's flow over the last days.
func (s *Store) FlowStats(ctx context.Context, wsID string, days int) (Flow, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	f := Flow{Days: days}
	since := time.Now().AddDate(0, 0, -days)

	rows, err := s.pool.Query(ctx, `
		SELECT runner, count(*), count(*) FILTER (WHERE verdict = 'passed'),
		       coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM finished_at - started_at) / 60), 0),
		       coalesce(sum(tokens_total), 0), coalesce(sum(cost_usd), 0)::float8
		  FROM runs
		 WHERE workspace_id = $1 AND coalesce(kind, 'work') = 'work' AND started_at >= $2 AND finished_at IS NOT NULL
		 GROUP BY runner ORDER BY count(*) DESC`, wsID, since)
	if err != nil {
		return f, err
	}
	for rows.Next() {
		var r RunnerFlow
		if err := rows.Scan(&r.Runner, &r.Runs, &r.Passed, &r.MedianMinutes, &r.Tokens, &r.CostUSD); err != nil {
			rows.Close()
			return f, err
		}
		if r.Runs > 0 {
			r.PassRate = float64(r.Passed) / float64(r.Runs)
		}
		f.Runners = append(f.Runners, r)
	}
	rows.Close()

	if err := s.pool.QueryRow(ctx, `
		WITH t AS (
		  SELECT issue_id, count(*) att, min(started_at) s, max(finished_at) FILTER (WHERE verdict = 'passed') f
		    FROM runs
		   WHERE workspace_id = $1 AND coalesce(kind, 'work') = 'work' AND started_at >= $2
		   GROUP BY issue_id HAVING bool_or(verdict = 'passed'))
		SELECT count(*), coalesce(avg(att), 0)::float8,
		       coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM f - s) / 3600), 0)::float8
		  FROM t`, wsID, since).Scan(&f.TicketsPassed, &f.AttemptsPerPassed, &f.MedianTicketHours); err != nil {
		return f, err
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE kind = 'code'),
		       coalesce(avg(CASE WHEN verdict = 'pass' THEN 1.0 ELSE 0 END) FILTER (WHERE kind = 'code' AND round = 1), 0)::float8,
		       coalesce(sum(jsonb_array_length(jsonb_path_query_array(findings, '$[*] ? (@.severity == "blocking")'))) FILTER (WHERE kind = 'code'), 0),
		       count(*) FILTER (WHERE kind = 'ticket'),
		       coalesce(avg(CASE WHEN verdict = 'pass' THEN 1.0 ELSE 0 END) FILTER (WHERE kind = 'ticket'), 0)::float8
		  FROM reviews WHERE workspace_id = $1 AND created_at >= $2`, wsID, since).Scan(
		&f.CodeReviews, &f.CodeFirstPassRate, &f.BlockingFound, &f.TicketReviews, &f.TicketQualifyRate); err != nil {
		return f, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT count(*), coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM now() - i.updated_at) / 3600), 0)::float8
		  FROM issues i JOIN workflow_states st ON st.id = i.state_id
		 WHERE i.workspace_id = $1 AND st.name = 'In Review'`, wsID).Scan(&f.WaitingForReview, &f.WaitingHoursMedian)
	return f, err
}
