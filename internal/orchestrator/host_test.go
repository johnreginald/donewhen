package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
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
	if len(hs) != len(models.Harnesses) {
		t.Errorf("heartbeat reported %d harnesses, want all %d", len(hs), len(models.Harnesses))
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

// drainRunner answers after a pause, unless its context ends first.
type drainRunner struct {
	wait    time.Duration
	started chan struct{}
}

func (drainRunner) Name() string                                       { return "claude" }
func (drainRunner) Run(context.Context, RunRequest) (RunResult, error) { return RunResult{}, nil }
func (s drainRunner) Ask(ctx context.Context, _, _ string) (string, float64, error) {
	close(s.started)
	select {
	case <-time.After(s.wait):
		return "hello", 0, nil
	case <-ctx.Done():
		return "", 0, ctx.Err()
	}
}
func (s drainRunner) AskIn(ctx context.Context, m, p, _ string) (string, float64, error) {
	return s.Ask(ctx, m, p)
}

// Stopping a host to update it lets the job in hand finish, up to Drain; a
// job that runs past it is ended.
func TestHostFinishesItsJobWhenStopped(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drain time.Duration
		want  string
	}{
		{"finishes", time.Minute, "succeeded"},
		{"past the drain", 30 * time.Millisecond, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			claimed := make(chan struct{})
			started := make(chan struct{})
			var finished map[string]any
			agent := models.Agent{ID: "ag-1", Name: "Engineer", Harness: "claude", Model: "haiku"}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/hosts/heartbeat", func(w http.ResponseWriter, r *http.Request) {})
			once := sync.Once{}
			mux.HandleFunc("POST /api/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
				served := false
				once.Do(func() {
					served = true
					json.NewEncoder(w).Encode(ClaimedJob{Job: models.Job{ID: "job-00001", Kind: "test_env"}, Agent: &agent})
					close(claimed)
				})
				if !served {
					w.WriteHeader(http.StatusNoContent)
				}
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
				Runners: RunnerSet{"claude": drainRunner{wait: 400 * time.Millisecond, started: started}},
				Probe: func(_ context.Context, harness string, r Runner) models.HarnessStatus {
					return models.HarnessStatus{Harness: harness, Installed: r != nil, Ready: r != nil}
				},
				Poll: 10 * time.Millisecond, Heartbeat: time.Hour, Drain: tc.drain,
			}
			ctx, stop := context.WithCancel(context.Background())
			ran := make(chan error, 1)
			go func() { ran <- h.Run(ctx) }()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("the job never started")
			}
			stop() // the host is told to stop mid-job
			select {
			case <-ran:
			case <-time.After(3 * time.Second):
				t.Fatal("the host did not stop")
			}
			mu.Lock()
			defer mu.Unlock()
			if finished == nil || finished["status"] != tc.want {
				t.Errorf("job ended %v, want %s", finished["status"], tc.want)
			}
		})
	}
}

// A person stopping a job from Raenil ends it on the host, reported as
// canceled rather than failed.
func TestHostStopsAJobWhenAsked(t *testing.T) {
	var mu sync.Mutex
	var finished map[string]any
	stopAsked := false
	started := make(chan struct{})
	agent := models.Agent{ID: "ag-1", Name: "Engineer", Harness: "claude", Model: "haiku"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/hosts/heartbeat", func(w http.ResponseWriter, r *http.Request) {})
	once := sync.Once{}
	mux.HandleFunc("POST /api/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		served := false
		once.Do(func() {
			served = true
			json.NewEncoder(w).Encode(ClaimedJob{Job: models.Job{ID: "job-00001", Kind: "test_env"}, Agent: &agent})
		})
		if !served {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(models.Job{ID: "job-00001", Status: "claimed", StopRequested: stopAsked})
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
		Runners: RunnerSet{"claude": drainRunner{wait: 10 * time.Second, started: started}},
		Probe: func(_ context.Context, harness string, r Runner) models.HarnessStatus {
			return models.HarnessStatus{Harness: harness, Installed: r != nil, Ready: r != nil}
		},
		Poll: 10 * time.Millisecond, Heartbeat: time.Hour, StopCheck: 20 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go h.Run(ctx)
	<-started
	mu.Lock()
	stopAsked = true
	mu.Unlock()
	for ctx.Err() == nil {
		mu.Lock()
		done := finished != nil
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if finished == nil || finished["status"] != "canceled" {
		t.Fatalf("job ended %v, want canceled", finished)
	}
}

// A host runs several jobs at once, up to Parallel.
func TestHostRunsJobsInParallel(t *testing.T) {
	var mu sync.Mutex
	next := 0
	running, peak := 0, 0
	agent := models.Agent{ID: "ag-1", Name: "Engineer", Harness: "claude", Model: "haiku"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/hosts/heartbeat", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("POST /api/jobs/claim", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if next >= 3 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next++
		json.NewEncoder(w).Encode(ClaimedJob{Job: models.Job{ID: fmt.Sprintf("job-%05d", next), Kind: "test_env"}, Agent: &agent})
	})
	mux.HandleFunc("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models.Job{Status: "claimed"})
	})
	finished := 0
	mux.HandleFunc("POST /api/jobs/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		finished++
		mu.Unlock()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	h := &Host{
		Name:    "mac",
		Clients: []*RaenilClient{{BaseURL: srv.URL, Token: "t", Workspace: "ws"}},
		Runners: RunnerSet{"claude": countingRunner{mu: &mu, running: &running, peak: &peak, wait: 300 * time.Millisecond}},
		Probe: func(_ context.Context, harness string, r Runner) models.HarnessStatus {
			return models.HarnessStatus{Harness: harness, Installed: r != nil, Ready: r != nil}
		},
		Poll: 10 * time.Millisecond, Heartbeat: time.Hour, Parallel: 2,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go h.Run(ctx)
	for ctx.Err() == nil {
		mu.Lock()
		done := finished == 3
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if finished != 3 {
		t.Fatalf("finished %d of 3 jobs", finished)
	}
	if peak != 2 {
		t.Errorf("at most %d ran at once, want 2 (Parallel)", peak)
	}
}

// countingRunner records how many answers are being worked on at once.
type countingRunner struct {
	mu            *sync.Mutex
	running, peak *int
	wait          time.Duration
}

func (countingRunner) Name() string                                       { return "claude" }
func (countingRunner) Run(context.Context, RunRequest) (RunResult, error) { return RunResult{}, nil }
func (c countingRunner) Ask(ctx context.Context, _, _ string) (string, float64, error) {
	c.mu.Lock()
	*c.running++
	if *c.running > *c.peak {
		*c.peak = *c.running
	}
	c.mu.Unlock()
	time.Sleep(c.wait)
	c.mu.Lock()
	*c.running--
	c.mu.Unlock()
	return "hello", 0, nil
}
func (c countingRunner) AskIn(ctx context.Context, m, p, _ string) (string, float64, error) {
	return c.Ask(ctx, m, p)
}
