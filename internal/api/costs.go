package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

// handleCosts sums runs over a period: mtd (the default), 7d, 30d or all.
func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	since := store.MonthStart(now)
	switch r.URL.Query().Get("range") {
	case "7d":
		since = now.AddDate(0, 0, -7)
	case "30d":
		since = now.AddDate(0, 0, -30)
	case "all":
		since = time.Time{}
	}
	c, err := s.store.Costs(r.Context(), ws(r), since)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, c)
}

// BudgetLine is an agent's caps against this month's use.
type BudgetLine struct {
	Agent models.Agent `json:"agent"`
	Usage store.Usage  `json:"usage"`
	Share float64      `json:"share"` // of the tighter cap; 0 when none
}

func (s *Server) handleBudgets(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	out := []BudgetLine{}
	for _, a := range agents {
		u, err := s.store.AgentMonthUsage(r.Context(), ws(r), a.ID, time.Now())
		if handleStoreErr(w, err) {
			return
		}
		out = append(out, BudgetLine{Agent: a, Usage: u, Share: store.BudgetShare(a.BudgetTokens, a.BudgetUSD, u)})
	}
	writeJSON(w, 200, out)
}

// checkBudget runs after each of an agent's runs, as Paperclip's budgets do:
// crossing 80% of a monthly cap is noted once, reaching 100% pauses the agent.
// A paused agent's queued work is not handed to a host, and nothing new is
// queued for it, until someone raises the cap or resumes it.
func (s *Server) checkBudget(ctx context.Context, wsID string, run models.Run) {
	if run.AgentID == nil {
		return
	}
	a, err := s.store.GetAgent(ctx, wsID, *run.AgentID)
	if err != nil || (a.BudgetTokens == 0 && a.BudgetUSD == 0) {
		return
	}
	u, err := s.store.AgentMonthUsage(ctx, wsID, a.ID, time.Now())
	if err != nil {
		return
	}
	now := store.BudgetShare(a.BudgetTokens, a.BudgetUSD, u)
	before := u
	before.Tokens -= int64(run.Tokens.Total)
	before.CostUSD -= run.CostUSD
	was := store.BudgetShare(a.BudgetTokens, a.BudgetUSD, before)

	note := func(detail string) {
		_ = s.store.RecordActivity(ctx, wsID, models.Activity{
			IssueID: run.IssueID, IssueKey: run.IssueKey, IssueTitle: run.IssueTitle,
			Actor: auth.ActorAI, Kind: "budget", Detail: detail,
		})
	}
	switch {
	case now >= 1 && a.Status == "active":
		paused := "paused"
		if saved, err := s.store.UpdateAgent(ctx, wsID, a.ID, store.AgentInput{Status: &paused}); err == nil {
			note(fmt.Sprintf("%s paused: it reached its monthly budget (%.0f%%)", a.Name, now*100))
			s.bus.Publish(events.Event{Type: "agent.saved", WorkspaceID: wsID, Actor: auth.ActorAI, Agent: &saved})
		}
	case was < 0.8 && now >= 0.8:
		note(fmt.Sprintf("%s has used %.0f%% of its monthly budget", a.Name, now*100))
	}
}
