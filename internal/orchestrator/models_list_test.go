package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCodexModelsReadsTheCache(t *testing.T) {
	home := t.TempDir()
	os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(`{"models":[
		{"slug":"gpt-5.5","visibility":"list","priority":3},
		{"slug":"gpt-reserve","visibility":"hide","priority":1},
		{"slug":"gpt-5.6-sol","visibility":"list","priority":2}]}`), 0o600)
	if got, want := CodexModels(home), []string{"default", "gpt-5.6-sol", "gpt-5.5"}; !slices.Equal(got, want) {
		t.Errorf("models = %v, want %v", got, want)
	}
	if got := CodexModels(t.TempDir()); !slices.Equal(got, []string{"default"}) {
		t.Errorf("no cache: %v", got)
	}
}

func TestOpenCodeProvidersOnlyConnected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"all":[
			{"id":"opencode-go","models":{"glm-5.3-flash":{},"kimi-k2.6":{}}},
			{"id":"anthropic","models":{"claude-sonnet-4-6":{}}}],
			"connected":["opencode-go"]}`))
	}))
	defer srv.Close()
	p, err := (&OpenCodeRunner{BaseURL: srv.URL}).Providers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"opencode-go/glm-5.3-flash", "opencode-go/kimi-k2.6"}; !slices.Equal(p.Models, want) {
		t.Errorf("models = %v, want %v — a provider without a key must not be offered", p.Models, want)
	}
}

func TestClaudeModelsFromTheAPI(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("anthropic-beta") == "" {
			t.Errorf("headers: %v", r.Header)
		}
		w.WriteHeader(status)
		w.Write([]byte(`{"data":[{"id":"claude-opus-5-5"},{"id":"claude-sonnet-5"}]}`))
	}))
	defer srv.Close()
	defer func(u string) { anthropicModelsURL = u }(anthropicModelsURL)
	anthropicModelsURL = srv.URL

	want := []string{"default", "sonnet", "opus", "haiku", "claude-opus-5-5", "claude-sonnet-5"}
	if got := ClaudeModels(context.Background(), "tok"); !slices.Equal(got, want) {
		t.Errorf("models = %v, want %v", got, want)
	}
	if got := ClaudeModels(context.Background(), ""); !slices.Equal(got, claudeAliases) {
		t.Errorf("without a token: %v", got)
	}
	status = http.StatusForbidden
	if got := ClaudeModels(context.Background(), "tok"); !slices.Equal(got, claudeAliases) {
		t.Errorf("refused: %v, want the aliases", got)
	}
}
