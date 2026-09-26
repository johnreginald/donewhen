package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRedact(t *testing.T) {
	t.Setenv("MY_SERVICE_TOKEN", "hunter2-plain-value")
	in := strings.Join([]string{
		"key sk-ant-oat01-abcdefghijklmnop",
		"openai sk-proj-ABCDEFGHIJKLMNOPQRSTUV",
		"raenil raenil_0123456789abcdef0123456789abcdef0123456789a",
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

func TestStreamLogPostsRenderedLinesAsTheyArrive(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Lines []string }
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		got = append(got, b.Lines...)
		mu.Unlock()
	}))
	defer srv.Close()
	c := &RaenilClient{BaseURL: srv.URL, Token: "t", Workspace: "w"}

	raw, _ := os.ReadFile("testdata/claude/denied.jsonl")
	lines := strings.SplitAfter(string(raw), "\n")
	half := len(lines) / 2
	path := filepath.Join(t.TempDir(), "run.jsonl")
	os.WriteFile(path, []byte(strings.Join(lines[:half], "")), 0o644)

	stop := streamLog(context.Background(), c, "run-1", path, "claude")
	time.Sleep(1700 * time.Millisecond) // one tick sees the first half
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(strings.Join(lines[half:], ""))
	f.Close()
	stop()

	mu.Lock()
	defer mu.Unlock()
	text := strings.Join(got, "\n")
	if strings.Contains(text, `"type"`) {
		t.Errorf("raw JSON was streamed:\n%s", text)
	}
	if want := claudeReadable(path); strings.TrimSpace(text) != strings.TrimSpace(want) {
		t.Errorf("streamed transcript differs from the whole-file render:\ngot:\n%s\nwant:\n%s", text, want)
	}
}
