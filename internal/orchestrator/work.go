package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// WorkConfig tunes the attempt loop.
type WorkConfig struct {
	Triage TriagePolicy
	// StateBounced is where a ticket lands when it goes back to a human.
	//
	// Blocked rather than Aligning: the work was specified and attempted and has
	// a branch behind it, so sending it back to the spec column would both lose
	// that context and hide it among tickets nobody has started.
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
		w.StateBounced = "Blocked"
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

	// Claim the ticket here rather than only in the daemon. Two terminals
	// running the same ticket would otherwise each cut a worktree and a branch
	// and race to commit — and the hand-run path is the common one.
	if o.Leases != nil {
		lease, err := o.Leases.Acquire(ref)
		if err != nil {
			return Verdict{}, err
		}
		defer lease.Release()
		stop := make(chan struct{})
		defer close(stop)
		go lease.KeepAlive(stop)
	}

	var last Verdict
	var spec AttemptSpec
	var totalCost float64
	// Which criteria failed, and how often. A criterion that fails on every
	// attempt while the rest pass is usually an unsatisfiable check rather than
	// unfinished work, and saying so saves a human re-reading the diff.
	failures := map[int]int{}
	names := map[int]string{}
	attempts := 0

	// Earlier runs of this ticket keep their branches; this run's attempts
	// are numbered after them so their branches never collide.
	prior := o.priorSlots(ctx, ref)
	for attempt := 1; attempt <= wc.Triage.MaxAttempts; attempt++ {
		spec.Attempt, spec.Slot = attempt, prior+attempt

		v, ev, err := o.RunAttempt(ctx, ref, spec)
		if err != nil {
			return v, err
		}
		last = v
		totalCost += v.CostUSD
		attempts++
		for _, e := range ev {
			names[e.CriterionIndex] = e.CriterionText
		}
		for _, i := range v.Failed {
			failures[i]++
		}

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
			if hint := alwaysFailing(failures, names, attempts); hint != "" {
				d.Reason += "\n\n" + hint
			}
			// In handoff mode a reviewer is waiting for this. Bouncing it to a
			// human with needs-info would be telling the senior to go away.
			if o.Cfg.Handoff {
				o.logf("not bouncing: %s — handing to the reviewer instead", d.Reason)
				last.Next = "review"
				return last, nil
			}
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
				FromBranch:    v.Branch,
				FromBase:      v.Base,
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
	reason := fmt.Sprintf("%d attempts did not satisfy the criteria", wc.Triage.MaxAttempts)
	if hint := alwaysFailing(failures, names, attempts); hint != "" {
		reason += "\n\n" + hint
	}
	if o.Cfg.Handoff {
		o.logf("not bouncing: %s — handing to the reviewer instead", reason)
		last.Next = "review"
		return last, nil
	}
	if err := o.bounce(ctx, ref, wc, reason); err != nil {
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

// alwaysFailing names criteria that failed on every single attempt.
//
// Work that is simply hard fails intermittently; a check that is impossible
// fails identically every time. Naming the difference is the difference between
// a human re-reading the code and a human re-reading the command.
func alwaysFailing(failures map[int]int, names map[int]string, attempts int) string {
	if attempts < 2 {
		return ""
	}
	var stuck []string
	for idx, n := range failures {
		if n == attempts {
			stuck = append(stuck, fmt.Sprintf("  - %q", names[idx]))
		}
	}
	if len(stuck) == 0 {
		return ""
	}
	sort.Strings(stuck)
	return fmt.Sprintf("These failed on all %d attempts:\n%s\n\n"+
		"A criterion that never once passes is often the command rather than the code. "+
		"Check it exits 0 on a repository where the work is already done.",
		attempts, strings.Join(stuck, "\n"))
}

// priorSlots is the highest attempt number a ticket's branches already use in
// its repository, or 0.
func (o *Orchestrator) priorSlots(ctx context.Context, ref string) int {
	scoped, issue, err := o.forTicket(ctx, ref)
	if err != nil {
		return 0
	}
	prefix := "ticket/" + strings.ToLower(issue.Key) + "-attempt-"
	out, err := git(ctx, scoped.Cfg.withDefaults().Repo, "branch", "--list", prefix+"*", "--format=%(refname:short)")
	if err != nil {
		return 0
	}
	high := 0
	for _, line := range strings.Split(out, "\n") {
		if n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(line), prefix)); err == nil && n > high {
			high = n
		}
	}
	return high
}
