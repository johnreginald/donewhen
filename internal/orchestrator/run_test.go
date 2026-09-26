package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
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
	runs      []map[string]any // POST /api/runs bodies
	finished  []map[string]any // PATCH /api/runs/{id} bodies
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
	mux.HandleFunc("GET /api/issues/{id}/blockers", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"blockedBy": []any{}, "blocking": []any{}})
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

	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.runs = append(f.runs, body)
		f.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"id": "run-1"})
	})
	mux.HandleFunc("PATCH /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.finished = append(f.finished, body)
		f.mu.Unlock()
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	// Pinned, so Scoped short-circuits — the same path a single-workspace
	// token takes in production.
	return &RaenilClient{BaseURL: srv.URL, Token: "test", Workspace: "test-ws"}
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
	if len(f.runs) != 1 || f.runs[0]["runner"] != "fake" || f.runs[0]["issue"] != "iss-1" {
		t.Errorf("run records started = %v, want one for the fake runner", f.runs)
	}
	if len(f.finished) != 1 || f.finished[0]["status"] != "succeeded" || f.finished[0]["verdict"] != string(StatusPassed) {
		t.Errorf("run records finished = %v, want succeeded with the passing verdict", f.finished)
	} else if tok, _ := f.finished[0]["tokens"].(map[string]any); tok["total"] != float64(100) {
		t.Errorf("finished tokens = %v, want total 100", f.finished[0]["tokens"])
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
	// The agent finished its turn; the criteria are what failed.
	if len(f.finished) != 1 || f.finished[0]["status"] != "succeeded" || f.finished[0]["verdict"] != string(StatusFailed) {
		t.Errorf("run record = %v, want the attempt succeeded with a failed verdict", f.finished)
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

// unreadyRunner reports that it cannot run the requested model.
type unreadyRunner struct{ reason string }

func (unreadyRunner) Name() string { return "unready" }
func (unreadyRunner) Run(context.Context, RunRequest) (RunResult, error) {
	return RunResult{}, errors.New("should never be called")
}
func (r unreadyRunner) Ready(context.Context, string) error { return errors.New(r.reason) }

// A misconfigured agent must be discovered before the ticket is claimed. It was
// discovered at the point of use, which left tickets parked In Progress with an
// empty worktree and nothing to show.
func TestUnreadyRunnerNeverClaimsTheTicket(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, unreadyRunner{reason: "provider package not installed"})
	o.Cfg.Model = "opencode-go/some-model"

	_, _, err := o.RunAttempt(context.Background(), "TST-1", AttemptSpec{Attempt: 1})
	if err == nil {
		t.Fatal("expected a refusal before any work started")
	}
	if !strings.Contains(err.Error(), "provider package not installed") {
		t.Errorf("the refusal should name the real cause, got %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) != 0 {
		t.Errorf("an unusable runner must not move the ticket, states = %v", f.states)
	}
	if f.branch != "" {
		t.Errorf("an unusable runner must not record a branch, got %q", f.branch)
	}
}

// A runner that can work is not blocked by the check.
func TestReadyRunnerProceeds(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, fixRunner{})
	o.Cfg.Model = "cheap/model"

	v, _, err := o.RunAttempt(context.Background(), "TST-1", AttemptSpec{Attempt: 1})
	if err != nil {
		t.Fatalf("RunAttempt: %v", err)
	}
	if v.Status != StatusPassed {
		t.Errorf("status = %s", v.Status)
	}
}

// promptRunner keeps the prompt it was given, then does the job.
type promptRunner struct{ got *string }

func (promptRunner) Name() string { return "fake" }
func (p promptRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	*p.got = req.Prompt
	return fixRunner{}.Run(ctx, req)
}

func TestWorkPromptCarriesAgentInstructions(t *testing.T) {
	var prompt string
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, promptRunner{got: &prompt})
	o.Instructions = "Always run the tests first."
	if _, err := o.RunTicket(context.Background(), "TST-1", 1); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(prompt, "## Your instructions\n\nAlways run the tests first.") || !strings.Contains(prompt, "TST-1") {
		t.Errorf("prompt does not open with the agent's instructions:\n%s", prompt)
	}
}

// A handed-back worktree's review diff shows the worker's committed change,
// not only what was touched since.
func TestReviewDiffShowsTheWholeChange(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, fixRunner{})
	o.Cfg.Handoff = true
	if _, err := o.RunTicket(context.Background(), "TST-1", 1); err != nil {
		t.Fatal(err)
	}
	diff, err := o.ReviewDiff(context.Background(), "TST-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+export const add = (a:number,b:number) => a + b") {
		t.Errorf("review diff lacks the worker's change:\n%s", diff)
	}
}

// Running a handed-back ticket again numbers its attempt after the kept one,
// instead of failing on the branch that already exists.
func TestWorkAgainAfterHandoff(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, fixRunner{})
	o.Cfg.Handoff = true
	for i := 0; i < 2; i++ {
		if _, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: TriagePolicy{MaxAttempts: 1}}); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	out, _ := git(context.Background(), o.Cfg.Repo, "branch", "--list", "ticket/tst-1-attempt-*", "--format=%(refname:short)")
	if !strings.Contains(out, "ticket/tst-1-attempt-1") || !strings.Contains(out, "ticket/tst-1-attempt-2") {
		t.Errorf("branches = %q, want attempt-1 and attempt-2", out)
	}
}

// capturingRunner records the Cwd it was handed, and whether that directory
// existed AT THE MOMENT IT RAN — which is the only moment that matters. The
// run removes the worktree on its way out, so asking afterwards always says
// no.
type capturingRunner struct {
	cwd    *string
	usable *bool
}

func (capturingRunner) Name() string                           { return "capturing" }
func (capturingRunner) Ready(context.Context, string) error    { return nil }
func (capturingRunner) Health(context.Context) (string, error) { return "ok", nil }

func (r capturingRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	*r.cwd = req.Cwd
	st, err := os.Stat(req.Cwd)
	*r.usable = err == nil && st.IsDir()
	return RunResult{}, nil
}

// The worker's Cwd has to be an absolute path that exists, and until API-114
// it was neither whenever RunRoot was left at its default.
//
// RunRoot defaults to the RELATIVE ".orchestrator", and every test above
// passes t.TempDir(), which is absolute — so nothing here ever exercised the
// shipped default. Git hid it too: `git -C <repo> worktree add` resolves a
// relative path against the REPO, so the worktree really was created, in the
// right place, every time.
//
// What broke was the same string being handed to the worker, which resolves
// it against its own working directory. opencode does not refuse a directory
// that does not exist: it creates the session, accepts the prompt with a 204,
// records the user message, and returns an assistant message with zero parts,
// zero tokens and no error. The run then polls until it times out, so a
// silently dead worker is indistinguishable from a slow one.
func TestTheWorkerIsGivenAnAbsoluteWorktreeThatExists(t *testing.T) {
	// A relative RunRoot is the shipped default. Chdir so the run directory
	// it creates lands somewhere disposable rather than in the package.
	t.Chdir(t.TempDir())

	var cwd string
	var usable bool
	f := &fakeRaenil{criteria: passingCriteria()}
	o := &Orchestrator{
		Raenil: f.server(t),
		Runner: capturingRunner{cwd: &cwd, usable: &usable},
		Cfg:    Config{RunRoot: ".orchestrator", Repo: newRepo(t), BaseRef: "HEAD"},
	}

	if _, err := o.RunTicket(context.Background(), "TST-1", 1); err != nil {
		t.Fatalf("RunTicket: %v", err)
	}

	if cwd == "" {
		t.Fatal("the runner was never given a working directory")
	}
	if !filepath.IsAbs(cwd) {
		t.Fatalf("worker cwd %q is relative; the worker resolves it against its own "+
			"directory, not the repo, and lands somewhere that does not exist", cwd)
	}
	if !usable {
		t.Fatalf("worker cwd %q did not exist when the worker ran", cwd)
	}
}

// stepRunner does half the job on its first go and checks, on its second,
// that the first go's work is still there.
type stepRunner struct {
	calls   *int
	sawHalf *bool
}

func (stepRunner) Name() string { return "step" }
func (r stepRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	*r.calls++
	if *r.calls == 1 {
		_ = os.WriteFile(filepath.Join(req.Cwd, "src", "half.ts"), []byte("export const half = 1\n"), 0o644)
		return RunResult{}, nil
	}
	_, err := os.Stat(filepath.Join(req.Cwd, "src", "half.ts"))
	*r.sawHalf = err == nil
	_ = os.WriteFile(filepath.Join(req.Cwd, "src", "add.ts"),
		[]byte("export const add = (a:number,b:number) => a + b\n"), 0o644)
	return RunResult{}, nil
}

// A retry carries on from the failed attempt's work instead of starting
// over, and the checks still see the whole change.
func TestRetryContinuesFromThePreviousAttempt(t *testing.T) {
	crit := append(passingCriteria(), models.Criterion{ID: "cr-3", Body: "half exists", Kind: models.CriterionDeterministic,
		CheckSpec: json.RawMessage(`{"cmd":"test -f src/half.ts"}`)})
	f := &fakeRaenil{criteria: crit}
	calls, saw := 0, false
	o := newOrch(t, f, stepRunner{calls: &calls, sawHalf: &saw})
	o.Cfg.Handoff = true
	v, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: TriagePolicy{MaxAttempts: 2, EscalateAfter: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !saw {
		t.Fatalf("calls=%d, second attempt saw the first's work: %v", calls, saw)
	}
	if v.Status != StatusPassed {
		t.Fatalf("status = %s, failed %v", v.Status, v.Failed)
	}
	if v.DiffStat.Files != 2 {
		t.Errorf("the checks saw %d changed files, want both attempts' (2)", v.DiffStat.Files)
	}
}
