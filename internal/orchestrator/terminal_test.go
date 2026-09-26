package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAsksUser(t *testing.T) {
	for msg, want := range map[string]bool{
		"Hello! What's your favourite colour?":                            true,
		"Should I use a table or a column?\n":                             true,
		"Which one do you want: **A or B?**":                              true,
		"Done. I asked myself whether X? and chose Y.\n\nAll tests pass.": false,
		"Implemented the endpoint.":                                       false,
		"":                                                                false,
	} {
		if got := asksUser(msg); got != want {
			t.Errorf("asksUser(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestParseTurn(t *testing.T) {
	claude := parseTurn(`{"session_id":"s1","hook_event_name":"Stop","last_assistant_message":"hi"}`)
	if claude.SessionID != "s1" || claude.Message != "hi" {
		t.Errorf("claude turn: %+v", claude)
	}
	codex := parseTurn(`{"type":"agent-turn-complete","thread-id":"t1","input-messages":["do it"],"last-assistant-message":"done"}`)
	if codex.SessionID != "t1" || codex.Message != "done" || codex.Input != "do it" {
		t.Errorf("codex turn: %+v", codex)
	}
}

// Codex titles each session in a side thread and reports its turn too; only
// the worker's own thread counts.
func TestWorkerThreadFilter(t *testing.T) {
	dir := t.TempDir()
	s := &terminalSession{RunDir: dir, Expect: "Work MYINF-1."}
	lines := `{"thread-id":"title","input-messages":["Generate a title.\n\nUser prompt:\nWork MYINF-1."],"last-assistant-message":"{\"title\":\"x\"}"}
{"thread-id":"main","input-messages":["Work MYINF-1."],"last-assistant-message":"first"}
{"thread-id":"title","last-assistant-message":"late title"}
{"thread-id":"main","input-messages":["fix it"],"last-assistant-message":"second"}
`
	if err := os.WriteFile(s.TurnsPath(), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []string
	for {
		turn, ok := s.nextTurn()
		if !ok {
			break
		}
		got = append(got, turn.Message)
	}
	if !slices.Equal(got, []string{"first", "second"}) {
		t.Errorf("turns = %q, want the worker's thread only", got)
	}
}

// fakeTurns is a scripted session for the turn loop.
func fakeTurns(msgs ...string) (*terminalSession, *[]string) {
	sent := &[]string{}
	i := 0
	s := &terminalSession{
		WaitFn: func(ctx context.Context) (Turn, error) {
			if i >= len(msgs) {
				return Turn{}, errSessionEnded
			}
			i++
			return Turn{SessionID: "s", Message: msgs[i-1]}, nil
		},
		SendFn: func(ctx context.Context, text string) error {
			*sent = append(*sent, text)
			return nil
		},
	}
	return s, sent
}

func TestTerminalTurnsQuestionWaits(t *testing.T) {
	s, sent := fakeTurns("Which table should I use?", "Done.")
	checks, asked := 0, 0
	turn, err := runTerminalTurns(context.Background(), s,
		func(context.Context) (bool, string) { checks++; return true, "" },
		func(string) { asked++ })
	if err != nil || turn.Message != "Done." {
		t.Fatalf("turn=%+v err=%v", turn, err)
	}
	if asked != 1 || checks != 1 || len(*sent) != 0 {
		t.Errorf("asked=%d checks=%d sent=%d: a question must not be checked", asked, checks, len(*sent))
	}
}

func TestTerminalTurnsFailureTypedBack(t *testing.T) {
	s, sent := fakeTurns("first try", "second try")
	calls := 0
	turn, err := runTerminalTurns(context.Background(), s, func(context.Context) (bool, string) {
		calls++
		return calls > 1, "tests fail"
	}, nil)
	if err != nil || turn.Message != "second try" {
		t.Fatalf("turn=%+v err=%v", turn, err)
	}
	if !slices.Equal(*sent, []string{"tests fail"}) {
		t.Errorf("sent = %q, want the failures typed back once", *sent)
	}
}

func TestTerminalTurnsRoundsRunOut(t *testing.T) {
	msgs := make([]string, TerminalRounds+2)
	for i := range msgs {
		msgs[i] = "still broken"
	}
	s, sent := fakeTurns(msgs...)
	_, err := runTerminalTurns(context.Background(), s, func(context.Context) (bool, string) { return false, "fail" }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(*sent) != TerminalRounds {
		t.Errorf("failures typed back %d times, want %d", len(*sent), TerminalRounds)
	}
}

func TestClaudeInteractiveArgs(t *testing.T) {
	r := &ClaudeRunner{MaxTurns: 40, MaxBudgetUSD: 2}
	args := r.interactiveArgs(RunRequest{Model: "claude/sonnet"}, "mcp.json", "s.json")
	for _, bad := range []string{"-p", "--output-format", "stream-json", "--verbose", "--max-turns", "--max-budget-usd"} {
		if slices.Contains(args, bad) {
			t.Errorf("interactive args keep print-mode flag %s: %v", bad, args)
		}
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--permission-mode auto", "--settings s.json", "--model sonnet", "--mcp-config mcp.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("interactive args lack %q: %v", want, args)
		}
	}
}

func TestPrepareClaudeConfigKeepsKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(path, []byte(`{"userID":"u","theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareClaudeConfig(dir, "/runs/worktrees/K-1-1"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	s := string(b)
	for _, want := range []string{`"userID": "u"`, `"theme": "light"`, `"hasCompletedOnboarding": true`, `"/runs/worktrees/K-1-1"`, `"hasTrustDialogAccepted": true`} {
		if !strings.Contains(s, want) {
			t.Errorf("config lacks %s:\n%s", want, s)
		}
	}
}
