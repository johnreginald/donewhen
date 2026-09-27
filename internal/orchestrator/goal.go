package orchestrator

import (
	"context"
	"errors"
	"time"
)

// The goal loop is how a work run knows it is done. Raenil does not rebuild
// an agent harness: each vendor's own CLI does the work, headless, and its own
// session carries on. What Raenil adds is the definition of done — the
// ticket's checks — and the next message when they fail.
//
// After each run the gating deterministic and policy checks run on the
// worktree. Pass: done. Fail: the same session resumes (claude --resume,
// codex exec resume, the OpenCode session, agy --conversation) with the
// failures as its next message. Stuck — the same checks failing on the same
// code turn after turn — ends the attempt, and triage decides what follows.

// GoalCheck runs the ticket's checks on the worktree as it stands.
type GoalCheck func(ctx context.Context) CheckResult

// CheckResult is one run of the checks.
type CheckResult struct {
	Pass bool
	// Feedback is what to tell the agent when they fail.
	Feedback string
	// State identifies what failed on what code. The same state turn after
	// turn means no progress.
	State string
}

// GoalStallTurns is how many turns in a row may end with the same checks
// failing on the same code before the attempt counts as stuck.
const GoalStallTurns = 3

// GoalMaxTurns bounds the loop even while the failures keep changing.
const GoalMaxTurns = 12

// errGoalStuck names why a stuck attempt ended.
var errGoalStuck = errors.New("stuck: the same checks failed on the same code for several turns in a row")

// goalLoop carries a finished run on until the checks pass. first is the
// result of the run that already happened; the result returned covers every
// turn — cost, tokens and time added up, the last answer and session kept.
func (o *Orchestrator) goalLoop(ctx context.Context, runner Runner, req RunRequest, first RunResult, firstErr error,
	check GoalCheck, issueID string) (RunResult, error) {
	res, err := first, firstErr
	stalled, prev := 0, ""
	maxTurns := o.Cfg.GoalTurns
	if maxTurns <= 0 {
		maxTurns = GoalMaxTurns
	}
	for turn := 1; ; turn++ {
		// A run that errored, timed out, or stopped to ask the user is not
		// resumed: those go to triage as they are.
		if err != nil || res.Aborted || len(res.Questions) > 0 || ctx.Err() != nil {
			return res, err
		}
		r := check(ctx)
		if r.Pass {
			o.logf("goal: checks pass after %d turn(s)", turn)
			return res, nil
		}
		if r.State != "" && r.State == prev {
			stalled++
		} else {
			stalled = 1
		}
		prev = r.State
		switch {
		case stalled >= GoalStallTurns:
			o.logf("goal: the same checks failed on the same code %d turns in a row — stuck", stalled)
			res.Stuck = true
			return res, nil
		case turn >= maxTurns:
			o.logf("goal: still failing after %d turns", turn)
			return res, nil
		case res.SessionID == "":
			o.logf("goal: %s reported no session to resume — the attempt ends here", runner.Name())
			return res, nil
		}
		o.logf("goal: checks fail — turn %d, resuming the session with the failures", turn+1)
		next := req
		next.Prompt, next.SessionID = r.Feedback, res.SessionID
		started := time.Now()
		more, merr := runner.Run(ctx, next)
		if o.AgentID != "" {
			more.Questions = append(more.Questions, o.questionsAskedSince(ctx, issueID, started)...)
		}
		res = addTurn(res, more)
		err = merr
		if err != nil {
			o.logf("runner error: %v", err)
		}
	}
}

// addTurn folds one more turn into an attempt's result.
func addTurn(total, turn RunResult) RunResult {
	total.CostUSD += turn.CostUSD
	total.NotionalCostUSD += turn.NotionalCostUSD
	total.Tokens += turn.Tokens
	total.Usage.Input += turn.Usage.Input
	total.Usage.Output += turn.Usage.Output
	total.Usage.CacheRead += turn.Usage.CacheRead
	total.Usage.CacheCreation += turn.Usage.CacheCreation
	total.Duration += turn.Duration
	total.DeniedTools = append(total.DeniedTools, turn.DeniedTools...)
	total.Questions = append(total.Questions, turn.Questions...)
	total.CostUnknown = total.CostUnknown || turn.CostUnknown
	if turn.SessionID != "" {
		total.SessionID = turn.SessionID
	}
	total.Answer, total.Exit, total.Aborted, total.AgentError = turn.Answer, turn.Exit, turn.Aborted, turn.AgentError
	if turn.Billing != "" {
		total.Billing = turn.Billing
	}
	return total
}
