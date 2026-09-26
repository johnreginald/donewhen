package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeSessionUsageCountsMessagesOnce(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	lines := `{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"assistant","message":{"id":"m2","usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}}
`
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	u, ok := claudeSessionUsage(p)
	if want := (TokenUsage{Input: 11, Output: 7, CacheRead: 103, CacheCreation: 24}); !ok || u != want {
		t.Errorf("usage = %+v (ok=%v), want %+v", u, ok, want)
	}
}

func TestCodexSessionUsageTakesLastTotal(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "09", "27")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":60,"output_tokens":5}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":300,"cached_input_tokens":200,"output_tokens":9}}}}
`
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-09-27T04-38-55-thread1.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	u, ok := codexSessionUsage(home, "thread1")
	if want := (TokenUsage{Input: 100, CacheRead: 200, Output: 9}); !ok || u != want {
		t.Errorf("usage = %+v (ok=%v), want %+v", u, ok, want)
	}
	if _, ok := codexSessionUsage(home, "other"); ok {
		t.Error("found usage for a thread that has no session file")
	}
}
