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

	"raenil/internal/models"
)

// fakeRaenil stands in for the tracker so the flow can be tested without one.
type fakeRaenil struct {
	mu        sync.Mutex
	criteria  []models.Criterion
	states    []string
	ticked    map[string]string // criterion id -> evidence ref
	comments  []string
	commits   []string
	documents []string
	branch    string
	labels    []string
	queue     []models.Issue
	state     string
}

func (f *fakeRaenil) server(t *testing.T) *RaenilClient {
	t.Helper()
	f.ticked = map[string]string{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /api/states", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]models.WorkflowState{
			{ID: "st-ready", Name: "Ready", Category: "unstarted"},
			{ID: "st-prog", Name: "In Progress", Category: "started"},
			{ID: "st-review", Name: "In Review", Category: "started"},
			{ID: "st-align", Name: "Aligning", Category: "unstarted"},
		})
	})
	mux.HandleFunc("GET /api/issues", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		// Once claimed, a ticket leaves the Ready queue.
		if f.state != "" && f.state != "Ready" {
			json.NewEncoder(w).Encode([]models.Issue{})
			return
		}
		json.NewEncoder(w).Encode(f.queue)
	})
	mux.HandleFunc("GET /api/issues/{id}", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models.Issue{
			ID: "iss-1", Key: "TST-1", Title: "fix add",
			DescriptionMD: "`add` subtracts instead of adding.",
		})
	})
	mux.HandleFunc("GET /api/issues/{id}/criteria", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		json.NewEncoder(w).Encode(f.criteria)
	})
	mux.HandleFunc("PATCH /api/criteria/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		ref, _ := body["evidenceRef"].(string)
		f.ticked[r.PathValue("id")] = ref
		f.mu.Unlock()
	})
	mux.HandleFunc("PATCH /api/issues/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		if s, ok := body["stateName"].(string); ok {
			f.states = append(f.states, s)
			f.state = s
		}
		if raw, ok := body["labelNames"].([]any); ok {
			f.labels = nil
			for _, v := range raw {
				if s, ok := v.(string); ok {
					f.labels = append(f.labels, s)
				}
			}
		}
		f.mu.Unlock()
	})
	mux.HandleFunc("PATCH /api/issues/{id}/dev", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.branch, _ = body["gitBranch"].(string)
		f.mu.Unlock()
	})
	mux.HandleFunc("POST /api/issues/{id}/comments", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		s, _ := body["bodyMd"].(string)
		f.comments = append(f.comments, s)
		f.mu.Unlock()
	})
	mux.HandleFunc("POST /api/issues/{id}/commits", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		s, _ := body["sha"].(string)
		f.commits = append(f.commits, s)
		f.mu.Unlock()
	})
	mux.HandleFunc("POST /api/documents", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		s, _ := body["bodyMd"].(string)
		f.documents = append(f.documents, s)
		f.mu.Unlock()
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &RaenilClient{BaseURL: srv.URL, Token: "test"}
}

// fixRunner simulates a worker that actually does the job.
type fixRunner struct{ questions []string }

func (fixRunner) Name() string { return "fake" }
func (r fixRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	_ = os.WriteFile(filepath.Join(req.Cwd, "src", "add.ts"),
		[]byte("export const add = (a:number,b:number) => a + b\n"), 0o644)
	return RunResult{SessionID: "ses_fake", CostUSD: 0.01, Tokens: 100, Questions: r.questions}, nil
}

// idleRunner changes nothing, so the criteria must fail.
type idleRunner struct{}

func (idleRunner) Name() string                                       { return "idle" }
func (idleRunner) Run(context.Context, RunRequest) (RunResult, error) { return RunResult{}, nil }

func passingCriteria() []models.Criterion {
	return []models.Criterion{
		{ID: "cr-1", Body: "add.ts adds", Kind: models.CriterionDeterministic,
			CheckSpec: json.RawMessage(`{"cmd":"grep -q 'a + b' src/add.ts"}`)},
		{ID: "cr-2", Body: "stays in src", Kind: models.CriterionPolicy,
			CheckSpec: json.RawMessage(`{"policy":"paths_within","args":["src/**"]}`)},
	}
}

func newOrch(t *testing.T, f *fakeRaenil, r Runner) *Orchestrator {
	t.Helper()
	return &Orchestrator{
		Raenil: f.server(t),
		Runner: r,
		Cfg:    Config{RunRoot: t.TempDir(), Repo: newRepo(t), BaseRef: "HEAD"},
	}
}

func TestRunTicketPassingFlow(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, fixRunner{})

	v, err := o.RunTicket(context.Background(), "TST-1", 1)
	if err != nil {
		t.Fatalf("RunTicket: %v", err)
	}
	if v.Status != StatusPassed {
		t.Fatalf("status=%s failed=%v", v.Status, v.Failed)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) != 2 || f.states[0] != "In Progress" || f.states[1] != "In Review" {
		t.Errorf("state transitions = %v, want [In Progress In Review]", f.states)
	}
	if len(f.ticked) != 2 {
		t.Errorf("both criteria should be ticked, got %v", f.ticked)
	}
	for id, ref := range f.ticked {
		if !strings.Contains(ref, "evidence.json#") {
			t.Errorf("criterion %s ticked without an evidence reference: %q", id, ref)
		}
	}
	if len(f.commits) != 1 {
		t.Errorf("a passing attempt should link its commit, got %v", f.commits)
	}
	if len(f.documents) != 1 || !strings.Contains(f.documents[0], "## Evidence") {
		t.Errorf("expected an evidence artifact to be saved, got %v", f.documents)
	}
	if !strings.HasPrefix(f.branch, "ticket/tst-1") {
		t.Errorf("branch = %q", f.branch)
	}
}

func TestRunTicketFailingFlow(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, idleRunner{})

	v, err := o.RunTicket(context.Background(), "TST-1", 1)
	if err != nil {
		t.Fatalf("a failing ticket is a result, not an error: %v", err)
	}
	if v.Status != StatusFailed || v.Next != "retry" {
		t.Errorf("status=%s next=%s", v.Status, v.Next)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	// It must NOT be moved to review, and the failure must be explained.
	for _, s := range f.states {
		if s == "In Review" {
			t.Error("a failing attempt must never reach In Review")
		}
	}
	if len(f.comments) == 0 || !strings.Contains(f.comments[0], "add.ts adds") {
		t.Errorf("expected a comment naming the failing criterion, got %v", f.comments)
	}
	if len(f.commits) != 0 {
		t.Error("a failing attempt must not link a commit")
	}
}

func TestRunTicketRefusesUnusableChecklist(t *testing.T) {
	// A criterion whose spec cannot be decoded is a gate that cannot gate.
	f := &fakeRaenil{criteria: []models.Criterion{
		{ID: "cr-1", Body: "broken", Kind: models.CriterionDeterministic, CheckSpec: json.RawMessage(`{}`)},
	}}
	o := newOrch(t, f, fixRunner{})
	if _, err := o.RunTicket(context.Background(), "TST-1", 1); err == nil {
		t.Fatal("expected a refusal to start on an unreadable checklist")
	}
}

func TestRunTicketRefusesWhenNothingIsGating(t *testing.T) {
	f := &fakeRaenil{criteria: []models.Criterion{
		{ID: "cr-1", Body: "someone eyeballs it", Kind: models.CriterionManual},
	}}
	o := newOrch(t, f, fixRunner{})
	_, err := o.RunTicket(context.Background(), "TST-1", 1)
	if err == nil || !strings.Contains(err.Error(), "nothing an orchestrator can decide") {
		t.Fatalf("expected a refusal when no criterion is machine-checkable, got %v", err)
	}
}

func TestRunTicketEscalatesWorkerQuestions(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, fixRunner{questions: []string{"which library should I use?"}})

	if _, err := o.RunTicket(context.Background(), "TST-1", 1); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var found bool
	for _, c := range f.comments {
		if strings.Contains(c, "which library should I use?") {
			found = true
		}
	}
	if !found {
		t.Errorf("a worker's question must reach the ticket, got %v", f.comments)
	}
}

func TestBuildPromptShowsTheContract(t *testing.T) {
	cs, err := ParseCriteria(passingCriteria())
	if err != nil {
		t.Fatal(err)
	}
	p := BuildPrompt(models.Issue{Key: "TST-1", Title: "fix add", DescriptionMD: "it subtracts"}, cs)
	for _, want := range []string{
		"TST-1", "fix add", "it subtracts",
		"grep -q 'a + b' src/add.ts", // the exact command it is judged by
		"paths_within", "src/**",
		"Do not weaken tests",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, p)
		}
	}
}
