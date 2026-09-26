package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"raenil/internal/models"
)

// JudgeFunc asks a model about a judgment criterion. The core never supplies one;
// the judgment plane injects it. Returning ok=false flags the criterion as
// advisory-failed, which is surfaced to a human but never blocks a ticket.
type JudgeFunc func(ctx context.Context, c JudgmentCheck, d Diff, workDir string) (ok bool, detail string, costUSD float64, err error)

// Evaluator turns criteria into evidence. It decides pass/fail; no model does.
type Evaluator struct {
	// WorkDir is the worktree deterministic checks run in.
	WorkDir string
	// RepoDir is the repository the worktree came from. Its .env is loaded over
	// the inherited environment so a check sees the repository's own test
	// database rather than whichever one the daemon happened to be started
	// with. Empty leaves the environment untouched.
	RepoDir string
	// Dir is where evidence and logs are written.
	Dir *RunDir
	// Judge is optional. Without it, judgment criteria are recorded as unevaluated.
	Judge JudgeFunc
	// Shell runs a command string. Defaults to "sh -c".
	Shell []string
	// OnEvidence fires as each criterion is decided. It exists so a criterion is
	// ticked on the tracker the moment it is met, rather than in a batch at the
	// end: an interrupted run must leave real partial progress behind.
	OnEvidence func(Evidence)
}

func (e *Evaluator) shell() []string {
	if len(e.Shell) > 0 {
		return e.Shell
	}
	return []string{"sh", "-c"}
}

// runCommand executes one deterministic check, capturing combined output to
// logPath. Every command is bounded: an unbounded check wedges the daemon with
// no diagnostic, which is the failure mode this design exists to avoid.
func (e *Evaluator) runCommand(ctx context.Context, c DeterministicCheck, logPath string) (exit int, err error) {
	timeout := DefaultCheckTimeout
	if c.Timeout != "" {
		d, perr := time.ParseDuration(c.Timeout)
		if perr != nil {
			return -1, fmt.Errorf("bad timeout %q: %w", c.Timeout, perr)
		}
		timeout = d
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	f, err := os.Create(logPath)
	if err != nil {
		return -1, fmt.Errorf("create log: %w", err)
	}
	defer f.Close()

	sh := e.shell()
	cmd := exec.CommandContext(ctx, sh[0], append(sh[1:], c.Cmd)...)
	cmd.Dir = e.WorkDir
	// Appended, so the repository's own values win over the daemon's. See
	// repoEnv: one global TEST_DATABASE_URL across seven routed repositories
	// is how a passing suite gets reported as a failure.
	if extra := repoEnv(e.RepoDir); len(extra) > 0 {
		cmd.Env = append(os.Environ(), extra...)
	}
	cmd.Stdout = f
	cmd.Stderr = f
	// A worker's shell must never inherit an open stdin: OpenCode hangs forever
	// on a live pipe, silently and with no output. Same class of bug here.
	cmd.Stdin = nil

	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return -1, fmt.Errorf("timed out after %s", timeout)
	}
	var ee *exec.ExitError
	switch {
	case runErr == nil:
		return 0, nil
	case errors.As(runErr, &ee):
		return ee.ExitCode(), nil
	default:
		return -1, runErr
	}
}

// Evaluate runs every criterion and returns the evidence. Evidence is flushed
// after each criterion so an interrupted run leaves real partial progress.
func (e *Evaluator) Evaluate(ctx context.Context, criteria []ParsedCriterion, diff Diff) ([]Evidence, error) {
	ev := make([]Evidence, 0, len(criteria))
	flush := func() {
		if e.Dir != nil {
			_ = e.Dir.WriteEvidence(ev)
		}
	}

	for _, c := range criteria {
		start := time.Now()
		item := Evidence{
			CriterionIndex: c.Index,
			CriterionText:  c.Text,
			Kind:           c.Kind,
			Timestamp:      start.UTC(),
		}

		switch c.Kind {
		case models.CriterionManual:
			// A human owns this one. Not a pass, not a failure — not ours.
			item.Detail = "manual criterion — a human ticks this"

		case models.CriterionDeterministic:
			logName := fmt.Sprintf("crit-%d.log", c.Index)
			logPath := logName
			if e.Dir != nil {
				logPath = e.Dir.File(logName)
			}
			item.Cmd = c.Deterministic.Cmd
			exit, err := e.runCommand(ctx, *c.Deterministic, logPath)
			item.Exit = exit
			item.OutputPath = logName
			if sum, herr := hashFile(logPath); herr == nil {
				item.OutputSHA256 = sum
			}
			if err != nil {
				item.Err = err.Error()
				break
			}
			want := 0
			if c.Deterministic.ExpectExit != nil {
				want = *c.Deterministic.ExpectExit
			}
			item.Pass = exit == want
			if !item.Pass {
				item.Detail = fmt.Sprintf("exit %d, expected %d", exit, want)
			}

		case models.CriterionPolicy:
			item.Policy = c.Policy.Policy
			res, err := EvalPolicy(*c.Policy, diff)
			if err != nil {
				item.Err = err.Error()
				break
			}
			item.Pass, item.Detail = res.Pass, res.Detail

		case models.CriterionJudgment:
			if e.Judge == nil {
				item.Detail = "judgment criterion — no judge configured, not evaluated"
				break
			}
			ok, detail, cost, err := e.Judge(ctx, *c.Judgment, diff, e.WorkDir)
			item.CostUSD = cost
			if err != nil {
				item.Err = err.Error()
				break
			}
			item.Pass, item.Detail = ok, detail
		}

		item.DurationMS = time.Since(start).Milliseconds()
		ev = append(ev, item)
		flush()
		if e.OnEvidence != nil {
			e.OnEvidence(item)
		}
	}
	return ev, nil
}

// Summarise folds evidence into a Verdict. Only gating criteria — deterministic
// and policy — can fail a ticket. Judgment and advisory criteria are reported
// separately, and manual criteria are left to a human.
func Summarise(ticket string, attempt int, criteria []ParsedCriterion, ev []Evidence, diff Diff) Verdict {
	v := Verdict{
		Ticket:   ticket,
		Attempt:  attempt,
		Passed:   []int{},
		Failed:   []int{},
		DiffStat: diff.Stat(),
	}
	byIndex := make(map[int]Evidence, len(ev))
	for _, e := range ev {
		byIndex[e.CriterionIndex] = e
		// Evaluation spends money too. Counting only the worker is how a budget
		// ceiling comes to sit above the actual bill.
		v.CostUSD += e.CostUSD
	}

	for _, c := range criteria {
		e, ok := byIndex[c.Index]
		switch {
		case c.Kind == models.CriterionJudgment:
			// Flag both a judge that objected and a judge that could not answer.
			// An unevaluated judgment silently vanishing from the verdict is how
			// a reviewer comes to believe something was checked when it was not.
			if ok && !e.Pass && (e.Detail != "" || e.Err != "") {
				v.AdvisoryFlagged = append(v.AdvisoryFlagged, c.Index)
			}
		case c.Advisory():
			// An advisory check that did not pass — or did not run — is a
			// flag for the reviewer, never a failed run.
			if ok && e.Pass {
				v.Passed = append(v.Passed, c.Index)
			} else {
				v.AdvisoryFlagged = append(v.AdvisoryFlagged, c.Index)
			}
		case !c.Gating():
			// manual — not the orchestrator's to decide
		case !ok:
			v.Failed = append(v.Failed, c.Index)
		case e.Pass:
			v.Passed = append(v.Passed, c.Index)
		default:
			v.Failed = append(v.Failed, c.Index)
		}
	}

	switch {
	case len(v.Failed) == 0:
		v.Status, v.Next = StatusPassed, "review"
	default:
		v.Status, v.Next = StatusFailed, "retry"
	}
	return v
}
