package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"raenil/internal/models"
)

// helloRunner answers a test prompt the way a working harness would.
type helloRunner struct{ answer string }

func (helloRunner) Name() string                                       { return "claude" }
func (helloRunner) Run(context.Context, RunRequest) (RunResult, error) { return RunResult{}, nil }
func (h helloRunner) Ask(context.Context, string, string) (string, float64, error) {
	return h.answer, 0, nil
}
func (h helloRunner) AskIn(context.Context, string, string, string) (string, float64, error) {
	return h.answer, 0, nil
}

func TestHostRunsATestEnvJob(t *testing.T) {
	var mu sync.Mutex
	var beats []map[string]any
	var finished map[string]any
	claimed := false
	agent := models.Agent{ID: "ag-1", Name: "Engineer", Harness: "claude", Model: "haiku"}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/hosts/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		beats = append(beats, b)
		mu.Unlock()
	})
	mux.HandleFunc("POST /api/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Harnesses []string }
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		defer mu.Unlock()
		if claimed || len(b.Harnesses) == 0 || b.Harnesses[0] != "claude" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		claimed = true
		json.NewEncoder(w).Encode(ClaimedJob{Job: models.Job{ID: "job-00001", Kind: "test_env"}, Agent: &agent})
	})
	mux.HandleFunc("POST /api/jobs/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		json.NewDecoder(r.Body).Decode(&finished)
		mu.Unlock()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	h := &Host{
		Name:    "mac",
		Clients: []*RaenilClient{{BaseURL: srv.URL, Token: "t", Workspace: "ws"}},
		Runners: RunnerSet{"claude": helloRunner{answer: "hello"}},
		Probe: func(_ context.Context, harness string, r Runner) models.HarnessStatus {
			return models.HarnessStatus{Harness: harness, Installed: r != nil, Ready: r != nil}
		},
		Poll: 20 * time.Millisecond, Heartbeat: time.Hour,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go h.Run(ctx)

	for ctx.Err() == nil {
		mu.Lock()
		done := finished != nil
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(beats) == 0 {
		t.Fatal("no heartbeat was sent")
	}
	hs, _ := beats[0]["harnesses"].([]any)
	if len(hs) != 3 {
		t.Errorf("heartbeat reported %d harnesses, want all 3", len(hs))
	}
	if finished == nil || finished["status"] != "succeeded" || finished["host"] != "mac" {
		t.Fatalf("finish = %v, want succeeded from mac", finished)
	}
	res, _ := finished["result"].(map[string]any)
	if res["answer"] != "hello" || res["harness"] != "claude" {
		t.Errorf("result = %v", res)
	}
}

func TestHostFailsATestWhenTheModelMisbehaves(t *testing.T) {
	h := &Host{Name: "mac", Runners: RunnerSet{"claude": helloRunner{answer: "I cannot do that"}}}
	_, err := h.testEnv(context.Background(), ClaimedJob{Agent: &models.Agent{Harness: "claude"}})
	if err == nil {
		t.Fatal("a model that did not answer as asked passed the test")
	}
	if _, err := h.testEnv(context.Background(), ClaimedJob{Agent: &models.Agent{Harness: "codex"}}); err == nil {
		t.Fatal("a harness the host does not have passed the test")
	}
}

func TestAgentModel(t *testing.T) {
	for _, c := range []struct {
		a    models.Agent
		want string
	}{
		{models.Agent{Harness: "claude", Model: "sonnet"}, "claude/sonnet"},
		{models.Agent{Harness: "claude"}, "claude/default"},
		{models.Agent{Harness: "codex", Model: "codex/gpt-5"}, "codex/gpt-5"},
		{models.Agent{Harness: "opencode", Model: "opencode-go/glm-5.3-flash"}, "opencode-go/glm-5.3-flash"},
	} {
		if got := AgentModel(c.a); got != c.want {
			t.Errorf("AgentModel(%+v) = %q, want %q", c.a, got, c.want)
		}
	}
}

// slowRunner holds a test prompt until released, like a long run.
type slowRunner struct{ release chan struct{} }

func (slowRunner) Name() string                                       { return "claude" }
func (slowRunner) Run(context.Context, RunRequest) (RunResult, error) { return RunResult{}, nil }
func (s slowRunner) Ask(ctx context.Context, _, _ string) (string, float64, error) {
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return "hello", 0, nil
}
func (s slowRunner) AskIn(ctx context.Context, m, p, _ string) (string, float64, error) {
	return s.Ask(ctx, m, p)
}

func TestHostKeepsBeatingDuringAJob(t *testing.T) {
	var mu sync.Mutex
	var beats []bool // the started flag of each heartbeat
	claimed, finished := false, false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/hosts/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var b struct{ Started bool }
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		beats = append(beats, b.Started)
		mu.Unlock()
	})
	mux.HandleFunc("POST /api/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if claimed {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		claimed = true
		json.NewEncoder(w).Encode(ClaimedJob{Job: models.Job{ID: "job-00002", Kind: "test_env"},
			Agent: &models.Agent{Harness: "claude"}})
	})
	mux.HandleFunc("POST /api/jobs/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		finished = true
		mu.Unlock()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	release := make(chan struct{})
	h := &Host{
		Name:    "mac",
		Clients: []*RaenilClient{{BaseURL: srv.URL, Token: "t", Workspace: "ws"}},
		Runners: RunnerSet{"claude": slowRunner{release: release}},
		Probe: func(_ context.Context, harness string, r Runner) models.HarnessStatus {
			return models.HarnessStatus{Harness: harness, Ready: r != nil}
		},
		Poll: 10 * time.Millisecond, Heartbeat: 15 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { h.Run(ctx); close(done) }()

	// Let the job start, then count beats while it is held.
	for ctx.Err() == nil {
		mu.Lock()
		c := claimed
		mu.Unlock()
		if c {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	before := len(beats)
	mu.Unlock()
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	during := len(beats) - before
	mu.Unlock()
	close(release)
	for ctx.Err() == nil {
		mu.Lock()
		f := finished
		mu.Unlock()
		if f {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if during < 3 {
		t.Errorf("only %d heartbeats while a job ran: a long run would make the host look gone", during)
	}
	mu.Lock()
	defer mu.Unlock()
	if !finished {
		t.Error("the job never finished")
	}
	if len(beats) == 0 || !beats[0] {
		t.Error("the first heartbeat did not say the host started")
	}
	for i, s := range beats[1:] {
		if s {
			t.Errorf("heartbeat %d claimed a restart", i+2)
		}
	}
}
