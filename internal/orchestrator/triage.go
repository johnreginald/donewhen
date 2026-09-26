package orchestrator

import (
	"fmt"
	"strings"

	"raenil/internal/models"
)

// Action is what to do after an attempt.
type Action string

const (
	// ActionReview means a human takes it from here.
	ActionReview Action = "review"
	// ActionRetry means try again with the same model, carrying the failure.
	ActionRetry Action = "retry"
	// ActionEscalate means try again with a stronger model, carrying the failure.
	ActionEscalate Action = "escalate"
	// ActionBounce means stop and hand it back to a human.
	ActionBounce Action = "bounce"
)

// Decision is the triage outcome for one attempt.
type Decision struct {
	Action Action
	// Reason is written to the ticket, so a human can see why without digging.
	Reason string
	// Model is the model the next attempt should use; empty keeps the current one.
	Model string
	// ContinueSession carries the failed attempt's context into the next one.
	ContinueSession bool
}

// TriagePolicy decides what happens after a failed attempt.
//
// These rules are deliberately mechanical. Asking a model "should we try again?"
// would put an opinion in the one place that must stay predictable: the thing
// that spends money and decides when to stop.
type TriagePolicy struct {
	// MaxAttempts is the hard stop. After this many, the ticket goes to a human.
	MaxAttempts int
	// EscalateAfter is the attempt number after which a stronger model is used.
	// Zero disables escalation.
	EscalateAfter int
	// EscalateModel is that stronger model.
	EscalateModel string
}

// DefaultTriagePolicy is a sane starting point: two cheap tries, then escalate,
// then hand it back.
func DefaultTriagePolicy(escalateModel string) TriagePolicy {
	return TriagePolicy{MaxAttempts: 3, EscalateAfter: 2, EscalateModel: escalateModel}
}

// Decide reads a verdict and says what to do next.
func (p TriagePolicy) Decide(v Verdict, ev []Evidence) Decision {
	if v.Status == StatusPassed {
		return Decision{Action: ActionReview, Reason: "all gating criteria passed"}
	}

	// A question is not a failure to retry through. The worker is blocked on
	// something only a human can settle, and asking the same model again just
	// spends money to be asked the same question.
	if len(v.Questions) > 0 {
		return Decision{
			Action: ActionBounce,
			Reason: "the worker asked a question it could not proceed without: " +
				strings.Join(v.Questions, "; "),
		}
	}

	// A blocker found during the attempt: the work needs another ticket's
	// first, and no retry supplies it. The ticket waits and starts again on
	// its own once the blocker clears.
	if len(v.WaitingOn) > 0 {
		return Decision{
			Action: ActionBounce,
			Reason: "the work needs " + strings.Join(v.WaitingOn, ", ") + " first; the ticket waits for it",
		}
	}

	// A refused command is refused again on every retry. The missing piece is
	// a rule only a person can add, so stop here rather than pay for the same
	// refusal twice more.
	if len(v.DeniedTools) > 0 {
		return Decision{
			Action: ActionBounce,
			Reason: "the worker was refused commands it needed: " + strings.Join(v.DeniedTools, "; ") +
				". Allow them for the agent and run the ticket again.",
		}
	}

	// A worker that tried to weaken the tests has told us something about its
	// judgment. Retrying the same model invites the same shortcut, so this
	// escalates or stops — it never retries in place.
	if hacked, detail := rewardHack(ev); hacked {
		if p.canEscalate(v.Attempt) {
			return Decision{
				Action: ActionEscalate, Model: p.EscalateModel, ContinueSession: false,
				Reason: "the worker tried to weaken the tests (" + detail +
					"); escalating with a fresh session rather than retrying the same approach",
			}
		}
		return Decision{
			Action: ActionBounce,
			Reason: "the worker tried to weaken the tests (" + detail + ")",
		}
	}

	// Timed out, or produced nothing at all: the model is not up to this ticket,
	// so a plain retry is unlikely to differ. Escalate if there is anywhere to go.
	if v.Status == StatusBlocked || v.DiffStat.Files == 0 {
		reason := "the worker produced no changes"
		if v.Status == StatusBlocked {
			reason = "the attempt was blocked: " + v.Blocked
		}
		switch {
		case p.canEscalate(v.Attempt):
			return Decision{Action: ActionEscalate, Model: p.EscalateModel,
				ContinueSession: true, Reason: reason + "; escalating"}
		case v.Attempt < p.MaxAttempts:
			// Escalation is not available yet. A worker that stalled once still
			// deserves a plain retry rather than going straight to a human.
			return Decision{Action: ActionRetry, ContinueSession: true,
				Reason: reason + "; retrying"}
		default:
			return Decision{Action: ActionBounce, Reason: reason}
		}
	}

	if v.Attempt >= p.MaxAttempts {
		return Decision{
			Action: ActionBounce,
			Reason: fmt.Sprintf("%d attempts did not satisfy: %s",
				v.Attempt, strings.Join(failedNames(v, ev), ", ")),
		}
	}
	if p.canEscalate(v.Attempt) {
		return Decision{Action: ActionEscalate, Model: p.EscalateModel, ContinueSession: true,
			Reason: "escalating after attempt " + fmt.Sprint(v.Attempt)}
	}
	return Decision{Action: ActionRetry, ContinueSession: true,
		Reason: "retrying with the failure in context"}
}

func (p TriagePolicy) canEscalate(attempt int) bool {
	return p.EscalateAfter > 0 && p.EscalateModel != "" && attempt >= p.EscalateAfter
}

// rewardHack reports whether the attempt failed specifically by trying to
// neuter the tests, which is the failure mode worth treating differently.
func rewardHack(ev []Evidence) (bool, string) {
	for _, e := range ev {
		if e.Kind == models.CriterionPolicy && e.Policy == "tests_not_weakened" && !e.Pass {
			return true, e.Detail
		}
	}
	return false, ""
}

func failedNames(v Verdict, ev []Evidence) []string {
	byIndex := map[int]Evidence{}
	for _, e := range ev {
		byIndex[e.CriterionIndex] = e
	}
	var out []string
	for _, i := range v.Failed {
		out = append(out, byIndex[i].CriterionText)
	}
	return out
}

// BuildRepairPrompt tells the next attempt exactly what refused the last one.
// It deliberately quotes the mechanical detail rather than paraphrasing it: the
// worker should be arguing with the same evidence a human would see.
func BuildRepairPrompt(issue models.Issue, criteria []ParsedCriterion, v Verdict, ev []Evidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Attempt %d did not pass. Fix what the checks below refused.\n\n", v.Attempt)

	byIndex := map[int]Evidence{}
	for _, e := range ev {
		byIndex[e.CriterionIndex] = e
	}
	b.WriteString("## What failed\n\n")
	for _, i := range v.Failed {
		e := byIndex[i]
		detail := e.Detail
		if e.Err != "" {
			detail = e.Err
		}
		fmt.Fprintf(&b, "- **%s**\n", e.CriterionText)
		if e.Cmd != "" {
			fmt.Fprintf(&b, "  Ran `%s`, exit %d.\n", e.Cmd, e.Exit)
		}
		if strings.TrimSpace(detail) != "" {
			fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(detail))
		}
	}

	if len(v.Passed) > 0 {
		b.WriteString("\n## What already passes — do not break these\n\n")
		for _, i := range v.Passed {
			fmt.Fprintf(&b, "- %s\n", byIndex[i].CriterionText)
		}
	}

	b.WriteString("\n")
	b.WriteString(BuildPrompt(issue, criteria))
	return b.String()
}
