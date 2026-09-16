package orchestrator

import (
	"context"
	"strings"
	"testing"
)

type stubAsker struct {
	answer    string
	err       error
	gotPrompt string
}

func (s *stubAsker) Ask(_ context.Context, _ string, prompt string) (string, error) {
	s.gotPrompt = prompt
	return s.answer, s.err
}

func TestJudgeParsesVerdicts(t *testing.T) {
	cases := []struct {
		answer  string
		wantOK  bool
		wantErr bool
	}{
		{"PASS\nFollows the ADR.", true, false},
		{"pass — looks fine", true, false},
		{"FAIL\nBypasses the repository layer.", false, false},
		{"FAIL: wrong layer", false, false},
		// An answer that cannot be read has approved nothing.
		{"I think it's probably fine?", false, true},
		{"", false, true},
	}
	for _, c := range cases {
		j := NewJudge(&stubAsker{answer: c.answer}, "some/model")
		ok, reason, err := j(context.Background(), JudgmentCheck{Prompt: "q"}, Diff{})
		if (err != nil) != c.wantErr {
			t.Errorf("answer %q: err = %v, wantErr %v", c.answer, err, c.wantErr)
			continue
		}
		if err == nil && ok != c.wantOK {
			t.Errorf("answer %q: ok = %v, want %v (reason %q)", c.answer, ok, c.wantOK, reason)
		}
	}
}

func TestJudgePromptCarriesTheDiff(t *testing.T) {
	s := &stubAsker{answer: "PASS\nfine"}
	j := NewJudge(s, "some/model")
	d := Diff{Files: []DiffFile{{Path: "src/a.ts", Status: "modified", Insertions: 2, Patch: "+const a = 1"}}}
	if _, _, err := j(context.Background(), JudgmentCheck{Prompt: "conforms to ADR-42?"}, d); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"conforms to ADR-42?", "src/a.ts", "+const a = 1", "PASS or FAIL"} {
		if !strings.Contains(s.gotPrompt, want) {
			t.Errorf("judge prompt missing %q", want)
		}
	}
}

func TestJudgeNeedsAModel(t *testing.T) {
	j := NewJudge(&stubAsker{answer: "PASS"}, "")
	if _, _, err := j(context.Background(), JudgmentCheck{Prompt: "q"}, Diff{}); err == nil {
		t.Error("expected an error when no model is configured")
	}
}
