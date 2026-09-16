package orchestrator

import (
	"context"
	"fmt"
	"strings"
)

// Asker is anything that can answer a one-shot question. OpenCodeRunner
// implements it; a test can substitute a stub.
type Asker interface {
	Ask(ctx context.Context, model, prompt string) (answer string, costUSD float64, err error)
	// AskIn answers from inside a directory, with read-only tools available.
	// Judgment criteria routinely ask the reviewer to look at the files, so a
	// judge with no filesystem cannot answer the questions it is actually given.
	AskIn(ctx context.Context, model, prompt, cwd string) (answer string, costUSD float64, err error)
}

// maxJudgeDiffBytes caps how much diff is sent to a judge. A judgment call is
// worth a few cents, not a whole context window.
const maxJudgeDiffBytes = 40000

// NewJudge builds a JudgeFunc backed by a model.
//
// What comes back is advisory by construction — Summarise never lets a judgment
// criterion populate Verdict.Failed. This exists to surface opinions a human
// should look at, not to hand a model the power to block a ticket.
func NewJudge(a Asker, model string) JudgeFunc {
	return func(ctx context.Context, c JudgmentCheck, d Diff, workDir string) (bool, string, float64, error) {
		m := c.Model
		if m == "" {
			m = model
		}
		if m == "" {
			return false, "", 0, fmt.Errorf("no model configured for judgment criteria")
		}

		var answer string
		var cost float64
		var err error
		if workDir != "" {
			answer, cost, err = a.AskIn(ctx, m, judgePrompt(c, d), workDir)
		} else {
			answer, cost, err = a.Ask(ctx, m, judgePrompt(c, d))
		}
		if err != nil {
			return false, "", cost, err
		}
		ok, detail, perr := parseJudgement(answer)
		return ok, detail, cost, perr
	}
}

func judgePrompt(c JudgmentCheck, d Diff) string {
	var b strings.Builder
	b.WriteString("You are reviewing a code change against one specific question. ")
	b.WriteString("Answer on the first line with exactly PASS or FAIL, then one short sentence of reasoning.\n\n")
	b.WriteString("You are in the worktree being reviewed and may read files with the " +
		"read, glob and grep tools. You cannot modify anything.\n\n")
	fmt.Fprintf(&b, "## Question\n\n%s\n\n## Change\n\n", c.Prompt)

	var n int
	for _, f := range d.Files {
		fmt.Fprintf(&b, "### %s (%s, +%d/-%d)\n", f.Path, f.Status, f.Insertions, f.Deletions)
		if n+len(f.Patch) > maxJudgeDiffBytes {
			b.WriteString("[diff truncated]\n")
			break
		}
		n += len(f.Patch)
		b.WriteString("```diff\n")
		b.WriteString(f.Patch)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// parseJudgement reads the model's verdict.
//
// An unparseable answer is an error, not a silent pass. A judge that cannot be
// understood has not approved anything.
func parseJudgement(answer string) (bool, string, error) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return false, "", fmt.Errorf("judge returned nothing")
	}
	first := trimmed
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		first = trimmed[:i]
	}
	upper := strings.ToUpper(strings.TrimSpace(first))
	reason := strings.TrimSpace(strings.TrimPrefix(trimmed, first))
	if reason == "" {
		reason = first
	}

	switch {
	case strings.HasPrefix(upper, "PASS"):
		return true, reason, nil
	case strings.HasPrefix(upper, "FAIL"):
		return false, reason, nil
	default:
		return false, "", fmt.Errorf("judge did not answer PASS or FAIL: %q", first)
	}
}
