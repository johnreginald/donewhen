package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex's events become the same readable lines as Claude's: what it said,
// each command with its result, each file it changed, how a turn failed.
func TestCodexLineRendersEvents(t *testing.T) {
	events := []string{
		`{"type":"thread.started","thread_id":"t-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"i0","type":"reasoning","text":"thinking"}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"go test ./...","aggregated_output":"ok  pkg\n","exit_code":0,"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"command_execution","command":"make lint","aggregated_output":"boom","exit_code":2,"status":"failed"}}`,
		`{"type":"item.completed","item":{"id":"i3","type":"file_change","changes":[{"path":"api/go.mod","kind":"add"}],"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i4","type":"agent_message","text":"Done: module created."}}`,
		`{"type":"turn.failed","error":{"message":"unexpected status 401"}}`,
		`Reading prompt from stdin...`,
	}
	var got []string
	for _, e := range events {
		got = append(got, codexLine([]byte(e))...)
	}
	want := []string{
		"· session t-1 · codex",
		"→ shell go test ./...",
		"← ok pkg",
		"→ shell make lint",
		"✗ boom",
		"→ create api/go.mod",
		"Done: module created.",
		"✗ unexpected status 401",
		"· Reading prompt from stdin...",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// OpenCode's saved session becomes the same lines, without Raenil's own
// prompt, the model's reasoning, or step markers.
func TestOpencodeReadable(t *testing.T) {
	doc := `[
	 {"info":{"role":"user","sessionID":"ses_1"},"parts":[{"type":"text","text":"PROMPT FROM RAENIL"}]},
	 {"info":{"role":"assistant","sessionID":"ses_1"},"parts":[
	   {"type":"step-start"},
	   {"type":"reasoning","text":"hmm"},
	   {"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"ls web"},"output":"a\nb"}},
	   {"type":"tool","tool":"edit","state":{"status":"error","input":{"filePath":"web/x.ts"},"error":"no such file"}},
	   {"type":"text","text":"All set."},
	   {"type":"step-finish"}
	 ]}
	]`
	path := filepath.Join(t.TempDir(), "worker.log")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	got := opencodeReadable(path)
	want := []string{
		"· session ses_1 · opencode",
		"→ bash ls web",
		"← a b",
		"→ edit web/x.ts",
		"✗ no such file",
		"All set.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if tail := runLogTail(path, "opencode"); strings.Contains(tail, "PROMPT FROM RAENIL") || !strings.Contains(tail, "All set.") {
		t.Errorf("run log tail is not the rendered transcript: %q", tail)
	}
}

// A worktree's absolute path becomes a path in the repository.
func TestWorktreePrefixStripped(t *testing.T) {
	got := worktreePrefix.ReplaceAllString("→ read /private/tmp/x/hostruns/worktrees/RAE-25-1/src/pow.js", "")
	if got != "→ read src/pow.js" {
		t.Errorf("got %q", got)
	}
}

// A worker may read the tracker without asking — a ticket names the tickets it
// depends on — but may not propose new ones.
func TestWorkRunMayReadTheTracker(t *testing.T) {
	m := RaenilMCP("https://r", "tok", "agent-1", "ws", WorkMCPTools...)
	args := (&ClaudeRunner{}).args(RunRequest{MCP: m}, "mcp.json")
	allowed := ""
	for i, a := range args {
		if a == "--allowedTools" {
			allowed = args[i+1]
		}
	}
	for _, want := range []string{"mcp__raenil__get_issue", "mcp__raenil__list_comments", "mcp__raenil__ask_user", "mcp__raenil__add_blocker"} {
		if !strings.Contains(allowed, want) {
			t.Errorf("work run cannot call %s without asking: %s", want, allowed)
		}
	}
	if strings.Contains(allowed, "propose_tickets") {
		t.Errorf("a work run may propose tickets: %s", allowed)
	}
}

// Raenil's own tool names read as themselves; its tokens do not.
func TestRedactKeepsRaenilToolNames(t *testing.T) {
	if got := redact("→ raenil_get_issue {\"id\":\"MYINF-15\"}"); !strings.Contains(got, "raenil_get_issue") {
		t.Errorf("tool name redacted: %q", got)
	}
	if got := redact("token raenil_EXAMPLEENTaM0LddGo5TCCsuV6_33qiYz1LMW3-Volo"); strings.Contains(got, "EXAMPLEE") {
		t.Errorf("token survived: %q", got)
	}
}
