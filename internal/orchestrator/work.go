package orchestrator

import (
	"context"
	"fmt"
	"strings"
)

// WorkConfig tunes the attempt loop.
type WorkConfig struct {
	Triage TriagePolicy
	// StateBounced is where a ticket lands when it goes back to a human.
	StateBounced string
	// LabelBounced is the triage label applied on a bounce.
	LabelBounced string
	// MaxCostUSD stops the loop once an issue has cost this much across attempts.
	// Zero disables the check.
	MaxCostUSD float64
}

func (w WorkConfig) withDefaults() WorkConfig {
	if w.Triage.MaxAttempts == 0 {
		w.Triage.MaxAttempts = 3
	}
	if w.StateBounced == "" {
		w.StateBounced = "Aligning"
	}
	if w.LabelBounced == "" {
		w.LabelBounced = "needs-info"
	}
	return w
}

// Work drives a ticket through attempts until it passes or a human is needed.
//
// The loop always terminates: every path either passes, exhausts MaxAttempts,
// breaches the cost ceiling, or bounces. There is no branch that retries
// indefinitely, because the one thing an unattended loop must never do is burn
// money in a circle.
func (o *Orchestrator) Work(ctx context.Context, ref string, wc WorkConfig) (Verdict, error) {
	wc = wc.withDefaults()

	var last Verdict
	var spec AttemptSpec
	var totalCost float64

	for attempt := 1; attempt <= wc.Triage.MaxAttempts; attempt++ {
		spec.Attempt = attempt

		v, ev, err := o.RunAttempt(ctx, ref, spec)
		if err != nil {
			return v, err
		}
		last = v
		totalCost += v.CostUSD

		d := wc.Triage.Decide(v, ev)

		// The cost ceiling overrides any decision to keep going.
		if wc.MaxCostUSD > 0 && totalCost >= wc.MaxCostUSD && d.Action != ActionReview {
			d = Decision{Action: ActionBounce,
				Reason: fmt.Sprintf("cost ceiling reached: $%.4f spent across %d attempts",
					totalCost, attempt)}
		}
		o.logf("triage: %s — %s", d.Action, d.Reason)

		switch d.Action {
		case ActionReview:
			return v, nil

		case ActionBounce:
			if err := o.bounce(ctx, ref, wc, d.Reason); err != nil {
				o.logf("warning: bounce incomplete: %v", err)
			}
			last.Next = string(ActionBounce)
			return last, nil

		case ActionRetry, ActionEscalate:
			spec = AttemptSpec{
				Model:         d.Model,
				PriorVerdict:  &v,
				PriorEvidence: ev,
			}
			if d.ContinueSession {
				spec.SessionID = v.SessionID
			}
			if d.Action == ActionEscalate {
				o.logf("escalating to %s", d.Model)
			}
		}
	}

	// Falling out of the loop means the last attempt asked to continue but there
	// are no attempts left.
	if err := o.bounce(ctx, ref, wc,
		fmt.Sprintf("%d attempts did not satisfy the criteria", wc.Triage.MaxAttempts)); err != nil {
		o.logf("warning: bounce incomplete: %v", err)
	}
	last.Next = string(ActionBounce)
	return last, nil
}

// bounce hands a ticket back to a human: move it, label it, and say why.
//
// A daemon that stalls silently is worse than one that fails, so the reason is
// always written to the ticket even if the state change fails.
func (o *Orchestrator) bounce(ctx context.Context, ref string, wc WorkConfig, reason string) error {
	issue, err := o.Raenil.Issue(ctx, ref)
	if err != nil {
		return err
	}
	var problems []string
	if err := o.Raenil.Comment(ctx, issue.ID,
		"Handing this back.\n\n"+reason+"\n\nNo further attempts will run until a human updates the ticket."); err != nil {
		problems = append(problems, "comment: "+err.Error())
	}
	if err := o.Raenil.SetState(ctx, issue.ID, wc.StateBounced); err != nil {
		problems = append(problems, "state: "+err.Error())
	}
	if err := o.Raenil.AddLabel(ctx, issue.ID, wc.LabelBounced); err != nil {
		problems = append(problems, "label: "+err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
