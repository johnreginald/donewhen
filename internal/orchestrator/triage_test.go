package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"raenil/internal/models"
)

func policy() TriagePolicy {
	return TriagePolicy{MaxAttempts: 3, EscalateAfter: 2, EscalateModel: "big/model"}
}

func TestTriagePassedGoesToReview(t *testing.T) {
	d := policy().Decide(Verdict{Status: StatusPassed, Attempt: 1}, nil)
	if d.Action != ActionReview {
		t.Errorf("action = %s, want review", d.Action)
	}
}

// A question cannot be answered by retrying — only a human can settle it.
func TestTriageQuestionBouncesImmediately(t *testing.T) {
	v := Verdict{Status: StatusFailed, Attempt: 1, Questions: []string{"which library?"}}
	d := policy().Decide(v, nil)
	if d.Action != ActionBounce {
		t.Fatalf("action = %s, want bounce on attempt 1", d.Action)
	}
	if !strings.Contains(d.Reason, "which library?") {
		t.Errorf("the question should be the reason, got %q", d.Reason)
	}
}

// A refused command is refused again on every retry: the rule is missing,
// not the effort. Only a person can add the rule, so the first attempt that
// was refused stops the run instead of spending the remaining attempts.
func TestTriageRefusedCommandBouncesImmediately(t *testing.T) {
	v := Verdict{Status: StatusFailed, Attempt: 1, DeniedTools: []string{"Bash node -e x"}}
	d := policy().Decide(v, nil)
	if d.Action != ActionBounce {
		t.Fatalf("action = %s, want bounce on attempt 1", d.Action)
	}
	if !strings.Contains(d.Reason, "node -e x") {
		t.Errorf("the refused command should be the reason, got %q", d.Reason)
	}
	// Refusals on a run that passed anyway do not hold it back.
	if d := policy().Decide(Verdict{Status: StatusPassed, Attempt: 1, DeniedTools: v.DeniedTools}, nil); d.Action != ActionReview {
		t.Errorf("a passing run with refusals: action = %s, want review", d.Action)
	}
}

// A dependency found mid-run is waited for, not retried through.
func TestTriageWaitingOnABlockerStops(t *testing.T) {
	d := policy().Decide(Verdict{Status: StatusFailed, Attempt: 1, WaitingOn: []string{"MYINF-20"}}, nil)
	if d.Action != ActionBounce || !strings.Contains(d.Reason, "MYINF-20") {
		t.Errorf("decision = %s %q, want bounce naming the blocker", d.Action, d.Reason)
	}
}

// A worker that tried to weaken the tests must not be retried in place.
func TestTriageRewardHackNeverRetries(t *testing.T) {
	ev := []Evidence{{
		CriterionIndex: 0, Kind: models.CriterionPolicy,
		Policy: "tests_not_weakened", Pass: false, Detail: "test file deleted: a_test.go",
	}}
	v := Verdict{Status: StatusFailed, Attempt: 2, Failed: []int{0}, DiffStat: DiffStat{Files: 1}}

	d := policy().Decide(v, ev)
	if d.Action != ActionEscalate {
		t.Errorf("action = %s, want escalate", d.Action)
	}
	if d.ContinueSession {
		t.Error("a cheating session must not be continued — that carries the bad approach forward")
	}

	// With nowhere to escalate, it stops rather than retrying.
	noEscalate := TriagePolicy{MaxAttempts: 3}
	if d := noEscalate.Decide(v, ev); d.Action != ActionBounce {
		t.Errorf("action = %s, want bounce when escalation is unavailable", d.Action)
	}
}

func TestTriageBlockedAndEmptyDiffEscalate(t *testing.T) {
	blocked := Verdict{Status: StatusBlocked, Attempt: 2, Blocked: "worker timed out", DiffStat: DiffStat{Files: 1}}
	if d := policy().Decide(blocked, nil); d.Action != ActionEscalate {
		t.Errorf("blocked: action = %s, want escalate", d.Action)
	}
	empty := Verdict{Status: StatusFailed, Attempt: 2, DiffStat: DiffStat{Files: 0}}
	if d := policy().Decide(empty, nil); d.Action != ActionEscalate {
		t.Errorf("empty diff: action = %s, want escalate", d.Action)
	}
}

func TestTriageRetriesThenEscalatesThenBounces(t *testing.T) {
	v := func(n int) Verdict {
		return Verdict{Status: StatusFailed, Attempt: n, Failed: []int{0}, DiffStat: DiffStat{Files: 1}}
	}
	ev := []Evidence{{CriterionIndex: 0, CriterionText: "build passes"}}

	if d := policy().Decide(v(1), ev); d.Action != ActionRetry || !d.ContinueSession {
		t.Errorf("attempt 1: %+v, want retry continuing the session", d)
	}
	if d := policy().Decide(v(2), ev); d.Action != ActionEscalate || d.Model != "big/model" {
		t.Errorf("attempt 2: %+v, want escalate to big/model", d)
	}
	if d := policy().Decide(v(3), ev); d.Action != ActionBounce {
		t.Errorf("attempt 3: %+v, want bounce at max attempts", d)
	}
}

func TestBuildRepairPromptQuotesTheEvidence(t *testing.T) {
	cs, err := ParseCriteria(passingCriteria())
	if err != nil {
		t.Fatal(err)
	}
	ev := []Evidence{
		{CriterionIndex: 0, CriterionText: "add.ts adds", Cmd: "grep -q 'a + b' src/add.ts", Exit: 1},
		{CriterionIndex: 1, CriterionText: "stays in src", Pass: true},
	}
	v := Verdict{Attempt: 1, Failed: []int{0}, Passed: []int{1}}
	p := BuildRepairPrompt(models.Issue{Key: "TST-1", Title: "fix add"}, cs, v, ev)

	for _, want := range []string{
		"Attempt 1 did not pass",
		"add.ts adds",
		"grep -q 'a + b' src/add.ts", // the actual command, not a paraphrase
		"exit 1",
		"do not break these",
		"stays in src",
	} {
		if !strings.Contains(strings.ToLower(p), strings.ToLower(want)) {
			t.Errorf("repair prompt missing %q\n---\n%s", want, p)
		}
	}
}

// flakyRunner fails the first n attempts, then does the work.
type flakyRunner struct {
	mu      sync.Mutex
	failFor int
	calls   int
	models  []string
}

func (r *flakyRunner) Name() string { return "flaky" }
func (r *flakyRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	r.mu.Lock()
	r.calls++
	n := r.calls
	r.models = append(r.models, req.Model)
	r.mu.Unlock()
	if n > r.failFor {
		_ = os.WriteFile(filepath.Join(req.Cwd, "src", "add.ts"),
			[]byte("export const add = (a:number,b:number) => a + b\n"), 0o644)
	}
	return RunResult{SessionID: "ses_" + string(rune('a'+n)), CostUSD: 0.01}, nil
}

func TestWorkRetriesUntilItPasses(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	r := &flakyRunner{failFor: 1}
	o := newOrch(t, f, r)
	o.Cfg.Model = "cheap/model"

	v, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: policy()})
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if v.Status != StatusPassed {
		t.Fatalf("status = %s after retry, want passed", v.Status)
	}
	if r.calls != 2 {
		t.Errorf("expected 2 attempts, got %d", r.calls)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) == 0 || f.states[len(f.states)-1] != "In Review" {
		t.Errorf("states = %v, want to end In Review", f.states)
	}
}

func TestWorkEscalatesModelOnSecondAttempt(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	r := &flakyRunner{failFor: 2}
	o := newOrch(t, f, r)
	o.Cfg.Model = "cheap/model"
	o.Cfg.GoalTurns = 1 // one call per attempt: this is about triage between attempts

	if _, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: policy()}); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.models) != 3 {
		t.Fatalf("expected 3 attempts, got %d (%v)", len(r.models), r.models)
	}
	if r.models[0] != "cheap/model" || r.models[1] != "cheap/model" {
		t.Errorf("first two attempts should use the cheap model, got %v", r.models)
	}
	if r.models[2] != "big/model" {
		t.Errorf("third attempt should be escalated, got %q", r.models[2])
	}
}

func TestWorkBouncesAfterMaxAttempts(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, idleRunner{})
	o.Cfg.Model = "cheap/model"

	v, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: TriagePolicy{MaxAttempts: 2}})
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if v.Next != string(ActionBounce) {
		t.Errorf("next = %q, want bounce", v.Next)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) == 0 || f.states[len(f.states)-1] != "Blocked" {
		t.Errorf("a bounced ticket should land in Blocked, states = %v", f.states)
	}
	var labelled bool
	for _, l := range f.labels {
		if l == "needs-info" {
			labelled = true
		}
	}
	if !labelled {
		t.Errorf("a bounced ticket should carry needs-info, labels = %v", f.labels)
	}
	for _, s := range f.states {
		if s == "In Review" {
			t.Error("a bounced ticket must never reach In Review")
		}
	}
}

func TestWorkStopsAtCostCeiling(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	r := &flakyRunner{failFor: 99} // never succeeds
	o := newOrch(t, f, r)
	o.Cfg.Model = "cheap/model"
	o.Cfg.GoalTurns = 1 // one call per attempt: the ceiling is counted per attempt

	// Each attempt costs $0.01; a $0.005 ceiling must stop after the first.
	v, err := o.Work(context.Background(), "TST-1",
		WorkConfig{Triage: TriagePolicy{MaxAttempts: 10}, MaxCostUSD: 0.005})
	if err != nil {
		t.Fatal(err)
	}
	if v.Next != string(ActionBounce) {
		t.Errorf("next = %q, want bounce", v.Next)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls != 1 {
		t.Errorf("cost ceiling should stop the loop after 1 attempt, got %d", r.calls)
	}
}

func TestWorkBouncesOnWorkerQuestionWithoutRetrying(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, idleRunner2{questions: []string{"which database?"}})
	o.Cfg.Model = "cheap/model"

	if _, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: policy()}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var found bool
	for _, c := range f.comments {
		if strings.Contains(c, "which database?") {
			found = true
		}
	}
	if !found {
		t.Errorf("the question should reach the ticket, comments = %v", f.comments)
	}
}

// idleRunner2 changes nothing and asks a question.
type idleRunner2 struct{ questions []string }

func (idleRunner2) Name() string { return "idle2" }
func (r idleRunner2) Run(context.Context, RunRequest) (RunResult, error) {
	return RunResult{Questions: r.questions}, nil
}

var _ = json.Marshal

// A criterion that fails every single attempt is usually an impossible command,
// not unfinished work. The bounce has to say so, or a human re-reads the diff
// looking for a fault that is in the check.
func TestBounceNamesAlwaysFailingCriteria(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, idleRunner{})
	o.Cfg.Model = "cheap/model"

	if _, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: TriagePolicy{MaxAttempts: 2}}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(f.comments, "\n")
	if !strings.Contains(joined, "failed on all 2 attempts") {
		t.Errorf("bounce should name what never passed:\n%s", joined)
	}
	if !strings.Contains(joined, "often the command rather than the code") {
		t.Errorf("bounce should point at the command:\n%s", joined)
	}
}

func TestAlwaysFailingNeedsMoreThanOneAttempt(t *testing.T) {
	names := map[int]string{0: "build passes"}
	// One attempt proves nothing about whether the check is satisfiable.
	if got := alwaysFailing(map[int]int{0: 1}, names, 1); got != "" {
		t.Errorf("single attempt should not accuse the command, got %q", got)
	}
	// Failing twice out of two is the signal.
	if got := alwaysFailing(map[int]int{0: 2}, names, 2); !strings.Contains(got, "build passes") {
		t.Errorf("expected the criterion to be named, got %q", got)
	}
	// Failing once out of two is ordinary flakiness, not an impossible check.
	if got := alwaysFailing(map[int]int{0: 1}, names, 2); got != "" {
		t.Errorf("intermittent failure should not accuse the command, got %q", got)
	}
}
