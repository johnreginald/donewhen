package orchestrator

import (
	"context"
	"fmt"
	"testing"
)

// scriptedRunner answers each Run with the next scripted result and records
// what it was asked.
type scriptedRunner struct {
	results []RunResult
	asked   []RunRequest
}

func (s *scriptedRunner) Name() string { return "scripted" }
func (s *scriptedRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	s.asked = append(s.asked, req)
	if len(s.asked) > len(s.results) {
		return RunResult{SessionID: "s1", Tokens: 1}, nil
	}
	return s.results[len(s.asked)-1], nil
}

func TestGoalLoopResumesSessionUntilPass(t *testing.T) {
	o := &Orchestrator{}
	r := &scriptedRunner{results: []RunResult{{SessionID: "s1", Tokens: 10, Answer: "fixed"}}}
	checks := 0
	res, err := o.goalLoop(context.Background(), r, RunRequest{Cwd: "/wt", Model: "m"},
		RunResult{SessionID: "s1", Tokens: 5}, nil,
		func(context.Context) CheckResult {
			checks++
			return CheckResult{Pass: checks > 1, Feedback: "tests fail", State: "a"}
		}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.asked) != 1 || r.asked[0].SessionID != "s1" || r.asked[0].Prompt != "tests fail" || r.asked[0].Cwd != "/wt" {
		t.Fatalf("resume = %+v, want the same session given the failures in the same worktree", r.asked)
	}
	if res.Tokens != 15 || res.Answer != "fixed" || res.Stuck {
		t.Errorf("result = %+v, want tokens added up and the last answer", res)
	}
}

func TestGoalLoopStopsWhenStuck(t *testing.T) {
	o := &Orchestrator{}
	r := &scriptedRunner{}
	res, err := o.goalLoop(context.Background(), r, RunRequest{}, RunResult{SessionID: "s1"}, nil,
		func(context.Context) CheckResult { return CheckResult{Feedback: "fail", State: "same"} }, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Stuck || len(r.asked) != GoalStallTurns-1 {
		t.Errorf("stuck=%v after %d resumes, want stuck after %d", res.Stuck, len(r.asked), GoalStallTurns-1)
	}
}

func TestGoalLoopKeepsGoingWhileFailuresChange(t *testing.T) {
	o := &Orchestrator{}
	r := &scriptedRunner{}
	n := 0
	res, _ := o.goalLoop(context.Background(), r, RunRequest{}, RunResult{SessionID: "s1"}, nil,
		func(context.Context) CheckResult {
			n++
			return CheckResult{Pass: n == 6, Feedback: "fail", State: fmt.Sprint(n)}
		}, "")
	if res.Stuck || len(r.asked) != 5 {
		t.Errorf("stuck=%v after %d resumes, want 5 resumes then a pass", res.Stuck, len(r.asked))
	}
}

func TestGoalLoopLeavesQuestionsAndErrorsToTriage(t *testing.T) {
	o := &Orchestrator{}
	for name, first := range map[string]RunResult{
		"question":   {SessionID: "s1", Questions: []string{"which table?"}},
		"timeout":    {SessionID: "s1", Aborted: true},
		"no session": {},
	} {
		r := &scriptedRunner{}
		o.goalLoop(context.Background(), r, RunRequest{}, first, nil,
			func(context.Context) CheckResult { return CheckResult{Feedback: "fail", State: "x"} }, "")
		if len(r.asked) != 0 {
			t.Errorf("%s: resumed %d times, want none", name, len(r.asked))
		}
	}
}
