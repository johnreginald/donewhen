package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeToken = "sk-ant-oat01-abcdefghijklmnopqrstuvwxyz0123456789"

func TestClaudeTokenIsStoredPrivately(t *testing.T) {
	c := Connections{Dir: t.TempDir()}
	if err := c.SaveClaudeToken("sk-ant-api03-abcdefghijklmnopqrstuvwxyz"); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("an API key was accepted: %v", err)
	}
	if err := c.SaveClaudeToken("hello"); err == nil {
		t.Error("a malformed token was accepted")
	}
	if err := c.SaveClaudeToken("  " + fakeToken + "\n"); err != nil {
		t.Fatal(err)
	}
	tok, ok := c.ClaudeToken()
	if !ok || tok != fakeToken {
		t.Errorf("read back %q", tok)
	}
	fi, _ := os.Stat(c.claudeTokenPath())
	di, _ := os.Stat(filepath.Dir(c.claudeTokenPath()))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Errorf("token file %v, dir %v; want 0600 and 0700", fi.Mode().Perm(), di.Mode().Perm())
	}

	if err := c.PrepareCodexHome(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(c.CodexHome(), "config.toml"))
	if !strings.Contains(string(b), `cli_auth_credentials_store = "file"`) || c.CodexConnected() {
		t.Errorf("codex home: %q connected=%v", b, c.CodexConnected())
	}
}

func TestClaudeRunIsolatedOnAConnection(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-should-not-leak")
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/you/.claude")
	dir := t.TempDir()
	fixture, _ := filepath.Abs("testdata/claude/ok.jsonl")
	bin := filepath.Join(dir, "claude")
	os.WriteFile(bin, []byte("#!/bin/sh\nenv > \"$(dirname \"$0\")/env.txt\"\ncat >/dev/null\ncat "+fixture+"\n"), 0o755)
	cfg := filepath.Join(dir, "cfg")

	r := &ClaudeRunner{Bin: bin, OAuthToken: fakeToken, ConfigDir: cfg}
	if _, err := r.Run(context.Background(), RunRequest{Prompt: "x", Cwd: dir}); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env.txt"))
	for _, want := range []string{"CLAUDE_CODE_OAUTH_TOKEN=" + fakeToken, "CLAUDE_CONFIG_DIR=" + cfg} {
		if !strings.Contains(string(env), want) {
			t.Errorf("run env lacks %s", want)
		}
	}
	for _, leak := range []string{"sk-should-not-leak", "/home/you/.claude"} {
		if strings.Contains(string(env), leak) {
			t.Errorf("run env carries %s", leak)
		}
	}
	if fi, err := os.Stat(cfg); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("config dir: %v %v", err, fi)
	}
}

func TestVerifyClaudeToken(t *testing.T) {
	status := http.StatusUnauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeToken || r.Header.Get("anthropic-beta") == "" {
			t.Errorf("request headers: %v", r.Header)
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	defer func(u string) { oauthUsageURL = u }(oauthUsageURL)
	oauthUsageURL = srv.URL

	if err := VerifyClaudeToken(context.Background(), fakeToken); err == nil {
		t.Error("a refused token passed")
	}
	status = http.StatusOK
	if err := VerifyClaudeToken(context.Background(), fakeToken); err != nil {
		t.Errorf("an accepted token failed: %v", err)
	}
	status = http.StatusNotFound // a moved endpoint must not stop agents
	if err := VerifyClaudeToken(context.Background(), fakeToken); err != nil {
		t.Errorf("an unknown answer blocked the connection: %v", err)
	}
}
