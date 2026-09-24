package store

import (
	"context"
	"time"
)

// Usage is what runs consumed: tokens by kind, and money by how it was paid.
type Usage struct {
	Runs             int     `json:"runs"`
	SubscriptionRuns int     `json:"subscriptionRuns"`
	APIRuns          int     `json:"apiRuns"`
	Tokens           int64   `json:"tokens"`
	Input            int64   `json:"input"`
	CacheRead        int64   `json:"cacheRead"`
	CacheCreation    int64   `json:"cacheCreation"`
	Output           int64   `json:"output"`
	CostUSD          float64 `json:"costUsd"`         // metered: what was paid
	NotionalUSD      float64 `json:"notionalCostUsd"` // subscription usage at list price
}

// CostLine is usage attributed to one agent or ticket.
type CostLine struct {
	ID    string `json:"id"`
	Key   string `json:"key,omitempty"`
	Name  string `json:"name"`
	Usage Usage  `json:"usage"`
}

// Costs is the workspace's spend over a period.
type Costs struct {
	Since    time.Time  `json:"since"`
	Total    Usage      `json:"total"`
	ByAgent  []CostLine `json:"byAgent"`
	ByTicket []CostLine `json:"byTicket"`
}

const usageSelect = `count(*), count(*) FILTER (WHERE r.billing = 'subscription'), count(*) FILTER (WHERE r.billing = 'api'),
	coalesce(sum(r.tokens_total), 0), coalesce(sum(r.tokens_input), 0), coalesce(sum(r.tokens_cache_read), 0),
	coalesce(sum(r.tokens_cache_creation), 0), coalesce(sum(r.tokens_output), 0),
	coalesce(sum(r.cost_usd), 0)::float8, coalesce(sum(r.notional_cost_usd), 0)::float8`

func usageDest(u *Usage) []any {
	return []any{&u.Runs, &u.SubscriptionRuns, &u.APIRuns, &u.Tokens, &u.Input, &u.CacheRead, &u.CacheCreation,
		&u.Output, &u.CostUSD, &u.NotionalUSD}
}

// Costs sums runs started since a moment: in total, by agent, and the
// tickets that cost the most.
func (s *Store) Costs(ctx context.Context, wsID string, since time.Time) (Costs, error) {
	c := Costs{Since: since, ByAgent: []CostLine{}, ByTicket: []CostLine{}}
	if err := s.pool.QueryRow(ctx, `SELECT `+usageSelect+` FROM runs r WHERE r.workspace_id = $1 AND r.started_at >= $2`,
		wsID, since).Scan(usageDest(&c.Total)...); err != nil {
		return c, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT coalesce(a.id::text, ''), coalesce(a.name, r.runner), `+usageSelect+`
		FROM runs r LEFT JOIN agents a ON a.id = r.agent_id
		WHERE r.workspace_id = $1 AND r.started_at >= $2
		GROUP BY a.id, a.name, r.runner ORDER BY sum(r.tokens_total) DESC`, wsID, since)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var l CostLine
		if err := rows.Scan(append([]any{&l.ID, &l.Name}, usageDest(&l.Usage)...)...); err != nil {
			rows.Close()
			return c, err
		}
		c.ByAgent = append(c.ByAgent, l)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `
		SELECT i.id::text, i.key, i.title, `+usageSelect+`
		FROM runs r JOIN issues i ON i.id = r.issue_id
		WHERE r.workspace_id = $1 AND r.started_at >= $2
		GROUP BY i.id, i.key, i.title ORDER BY sum(r.tokens_total) DESC LIMIT 20`, wsID, since)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var l CostLine
		if err := rows.Scan(append([]any{&l.ID, &l.Key, &l.Name}, usageDest(&l.Usage)...)...); err != nil {
			return c, err
		}
		c.ByTicket = append(c.ByTicket, l)
	}
	return c, rows.Err()
}

// MonthStart is the start of the calendar month in UTC — the window budgets
// count in, as Paperclip's do.
func MonthStart(now time.Time) time.Time {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// AgentMonthUsage is what an agent's runs have used this month.
func (s *Store) AgentMonthUsage(ctx context.Context, wsID, agentID string, now time.Time) (Usage, error) {
	var u Usage
	err := s.pool.QueryRow(ctx, `SELECT `+usageSelect+` FROM runs r
		WHERE r.workspace_id = $1 AND r.agent_id::text = $2 AND r.started_at >= $3`,
		wsID, agentID, MonthStart(now)).Scan(usageDest(&u)...)
	return u, err
}

// BudgetShare is the larger of the agent's used fractions of its caps, or 0
// when it has none.
func BudgetShare(tokensCap int64, usdCap float64, u Usage) float64 {
	share := 0.0
	if tokensCap > 0 {
		share = float64(u.Tokens) / float64(tokensCap)
	}
	if usdCap > 0 && u.CostUSD/usdCap > share {
		share = u.CostUSD / usdCap
	}
	return share
}
