package orchestrator

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	t.Setenv("MY_SERVICE_TOKEN", "hunter2-plain-value")
	in := strings.Join([]string{
		"key sk-ant-oat01-abcdefghijklmnop",
		"openai sk-proj-ABCDEFGHIJKLMNOPQRSTUV",
		"raenil raenil_0123456789abcdef",
		"github ghp_ABCDEFGHIJKLMNOPQRSTUVWX",
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload",
		"env hunter2-plain-value",
		"ordinary words stay",
	}, "\n")
	out := redact(in)
	for _, secret := range []string{"sk-ant-oat01", "sk-proj-", "raenil_0123", "ghp_ABC", "eyJhbGci", "hunter2"} {
		if strings.Contains(out, secret) {
			t.Errorf("%q survived redaction:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "Bearer [redacted]") || !strings.Contains(out, "ordinary words stay") {
		t.Errorf("redaction damaged the text around secrets:\n%s", out)
	}
}

func TestClaudeReadable(t *testing.T) {
	got := claudeReadable("testdata/claude/denied.jsonl")
	for _, want := range []string{"· session", "→ Bash touch /tmp/raenil-outside.txt", "✗ ", "· success"} {
		if !strings.Contains(got, want) {
			t.Errorf("readable transcript lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"type"`) {
		t.Errorf("raw JSON leaked into the readable transcript:\n%s", got)
	}
}
