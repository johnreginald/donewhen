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

// fakeOpenCode is a stand-in for `opencode serve`, shaped from its OpenAPI spec.
type fakeOpenCode struct {
	mu sync.Mutex

	created     bool
	dispatched  map[string]any
	release     chan struct{} // closed to let the blocking wait return
	perms       []PermissionRequest
	questions   []QuestionRequest
	permReplies map[string]string
	rejectedQ   []string
	aborted     bool
	agentError  string
}

func newFake() *fakeOpenCode {
	return &fakeOpenCode{release: make(chan struct{}), permReplies: map[string]string{}}
}

func (f *fakeOpenCode) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /global/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"healthy": true, "version": "1.18.31"})
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.created = true
		f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"id": "ses_fake"})
	})
	mux.HandleFunc("POST /session/{id}/prompt_async", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.dispatched = body
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /session/{id}/abort", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.aborted = true
		f.mu.Unlock()
		json.NewEncoder(w).Encode(true)
	})
	mux.HandleFunc("GET /session/{id}", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": "ses_fake", "cost": 0.42,
			"tokens": map[string]any{"input": 100, "output": 50, "reasoning": 10},
		})
	})
	// Mirrors the real shape: metadata under "info", completion signalled by a
	// non-zero time.completed on the last assistant message.
	mux.HandleFunc("GET /session/{id}/message", func(w http.ResponseWriter, r *http.Request) {
		completed := float64(0)
		select {
		case <-f.release:
			completed = 1789532282968
		default:
		}
		info := map[string]any{
			"id": "msg_fake", "role": "assistant",
			"time":   map[string]any{"created": 1789532280780, "completed": completed},
			"finish": "stop", "cost": 0.42,
			"tokens": map[string]any{"input": 100, "output": 50, "reasoning": 10},
		}
		if f.agentError != "" {
			info["error"] = map[string]any{"message": f.agentError}
		}
		json.NewEncoder(w).Encode([]map[string]any{{"info": info, "parts": []any{}}})
	})
	mux.HandleFunc("GET /permission", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewEncoder(w).Encode(f.perms)
	})
	mux.HandleFunc("POST /permission/{id}/reply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.permReplies[r.PathValue("id")], _ = body["reply"].(string)
		f.perms = nil // answered
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /question", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewEncoder(w).Encode(f.questions)
	})
	mux.HandleFunc("POST /question/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.rejectedQ = append(f.rejectedQ, r.PathValue("id"))
		f.questions = nil
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newRunner(t *testing.T, f *fakeOpenCode) *OpenCodeRunner {
	t.Helper()
	srv := f.server(t)
	return &OpenCodeRunner{BaseURL: srv.URL, PollInterval: 10 * time.Millisecond}
}

func TestOpenCodeHappyPath(t *testing.T) {
	f := newFake()
	r := newRunner(t, f)
	close(f.release) // finish immediately

	log := filepath.Join(t.TempDir(), "worker.log")
	res, err := r.Run(context.Background(), RunRequest{
		Prompt: "fix the bug", Cwd: t.TempDir(),
		Model: "opencode-go/glm-5.3-flash", LogPath: log,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Exit != 0 || res.Aborted {
		t.Errorf("exit=%d aborted=%v, want 0/false", res.Exit, res.Aborted)
	}
	if res.CostUSD != 0.42 || res.Tokens != 160 {
		t.Errorf("cost=%v tokens=%d, want 0.42/160", res.CostUSD, res.Tokens)
	}
	if !f.created {
		t.Error("expected a session to be created")
	}
	// The model must reach opencode split into provider and model.
	m, _ := f.dispatched["model"].(map[string]any)
	if m["providerID"] != "opencode-go" || m["modelID"] != "glm-5.3-flash" {
		t.Errorf("model dispatched as %v", m)
	}
	if _, err := os.Stat(log); err != nil {
		t.Errorf("transcript should be written to disk: %v", err)
	}
}

func TestOpenCodeReusesSession(t *testing.T) {
	f := newFake()
	r := newRunner(t, f)
	close(f.release)

	res, err := r.Run(context.Background(), RunRequest{
		Prompt: "try again", Cwd: t.TempDir(), SessionID: "ses_previous",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.created {
		t.Error("a continued attempt must not create a new session — that loses the failed attempt's context")
	}
	if res.SessionID != "ses_previous" {
		t.Errorf("SessionID = %q, want ses_previous", res.SessionID)
	}
}

func TestOpenCodeRejectsModelWithoutProvider(t *testing.T) {
	f := newFake()
	r := newRunner(t, f)
	close(f.release)
	_, err := r.Run(context.Background(), RunRequest{Prompt: "x", Cwd: t.TempDir(), Model: "glm-5.3-flash"})
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Errorf("expected a provider-prefix error, got %v", err)
	}
}

func TestOpenCodePermissionPolicyIsEnforced(t *testing.T) {
	f := newFake()
	f.perms = []PermissionRequest{{ID: "per_1", SessionID: "ses_fake", Permission: "edit"}}
	r := newRunner(t, f)
	r.Permission = func(p PermissionRequest) (PermissionDecision, string) {
		return PermissionReject, "outside the ticket's module"
	}

	go func() { time.Sleep(80 * time.Millisecond); close(f.release) }()
	res, err := r.Run(context.Background(), RunRequest{Prompt: "x", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	f.mu.Lock()
	reply := f.permReplies["per_1"]
	f.mu.Unlock()
	if reply != "reject" {
		t.Errorf("permission reply = %q, want reject", reply)
	}
	if len(res.DeniedTools) != 1 {
		t.Errorf("denied tools should be reported to the reviewer, got %v", res.DeniedTools)
	}
}

// A daemon cannot answer a question. It must capture it and move on, never hang.
func TestOpenCodeQuestionIsCapturedAndRejected(t *testing.T) {
	f := newFake()
	f.questions = []QuestionRequest{{ID: "qst_1", SessionID: "ses_fake", Text: "which auth library?"}}
	r := newRunner(t, f)

	go func() { time.Sleep(80 * time.Millisecond); close(f.release) }()
	res, err := r.Run(context.Background(), RunRequest{Prompt: "x", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Questions) != 1 || res.Questions[0] != "which auth library?" {
		t.Errorf("question should become the escalation payload, got %v", res.Questions)
	}
	f.mu.Lock()
	rejected := len(f.rejectedQ)
	f.mu.Unlock()
	if rejected != 1 {
		t.Error("question should be rejected so the agent is unblocked")
	}
}

func TestOpenCodeTimeoutAborts(t *testing.T) {
	f := newFake() // never released
	r := newRunner(t, f)

	res, err := r.Run(context.Background(), RunRequest{
		Prompt: "x", Cwd: t.TempDir(), Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("a timeout is a result, not an error: %v", err)
	}
	if !res.Aborted || res.Exit != 124 {
		t.Errorf("aborted=%v exit=%d, want true/124", res.Aborted, res.Exit)
	}
	f.mu.Lock()
	aborted := f.aborted
	f.mu.Unlock()
	if !aborted {
		t.Error("the session must actually be aborted server-side, not just abandoned")
	}
}

func TestOpenCodeHealth(t *testing.T) {
	f := newFake()
	r := newRunner(t, f)
	v, err := r.Health(context.Background())
	if err != nil || v != "1.18.31" {
		t.Errorf("Health() = %q, %v", v, err)
	}
}
