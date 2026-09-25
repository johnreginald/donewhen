package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"raenil/internal/models"
)

func newDaemon(t *testing.T, f *fakeRaenil, r Runner) *Daemon {
	t.Helper()
	o := newOrch(t, f, r)
	o.Cfg.Model = "cheap/model"
	return &Daemon{
		Orch:   o,
		Leases: &LeaseManager{Dir: filepath.Join(o.Cfg.RunRoot, "leases"), TTL: 50 * time.Millisecond},
		Cfg: DaemonConfig{
			ReadyState:    "Ready",
			PollInterval:  20 * time.Millisecond,
			MaxConcurrent: 1,
			Work:          WorkConfig{Triage: TriagePolicy{MaxAttempts: 1}},
		},
	}
}

func queued() []models.Issue {
	return []models.Issue{{ID: "iss-1", Key: "TST-1", Title: "fix add"}}
}

func TestDaemonWorksTheQueueThenReleasesTheLease(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria(), queue: queued()}
	d := newDaemon(t, f, fixRunner{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	waitFor(t, 3*time.Second, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, s := range f.states {
			if s == "In Review" {
				return true
			}
		}
		return false
	}, "ticket to reach In Review")

	cancel()
	<-done

	// The lease must not outlive the work, or the ticket is stuck forever.
	held, err := d.Leases.Held()
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 {
		t.Errorf("leases should be released on shutdown, still held: %v", held)
	}
}

func TestDaemonHaltsOnKillSwitch(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria(), queue: queued()}
	d := newDaemon(t, f, fixRunner{})
	stop := filepath.Join(t.TempDir(), "STOP")
	d.Cfg.StopFile = stop
	if err := os.WriteFile(stop, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) != 0 {
		t.Errorf("no ticket should be touched while the kill switch is present, states = %v", f.states)
	}
}

func TestDaemonHaltsOnHourlyBudget(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria(), queue: queued()}
	d := newDaemon(t, f, fixRunner{})
	d.Cfg.MaxUSDPerHour = 0.005
	d.spend.add(0.01) // already over

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) != 0 {
		t.Errorf("an exhausted budget must stop work starting, states = %v", f.states)
	}
}

// Crash recovery: a ticket left In Progress by a dead daemon goes back on the queue.
func TestDaemonRecoversTicketsFromDeadHolder(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	d := newDaemon(t, f, fixRunner{})
	writeStaleLease(t, d.Leases, "TST-1", 999999, time.Now().Add(-time.Hour))

	if err := d.recover(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var reset bool
	for _, s := range f.states {
		if s == "Ready" {
			reset = true
		}
	}
	if !reset {
		t.Errorf("an abandoned ticket should return to Ready, states = %v", f.states)
	}
	if len(f.comments) == 0 {
		t.Error("recovery should say on the ticket what happened")
	}
	held, _ := d.Leases.Held()
	if len(held) != 0 {
		t.Errorf("the dead lease should be gone, still held: %v", held)
	}
}

func TestDaemonGCsOrphanedWorktrees(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	d := newDaemon(t, f, fixRunner{})

	wtRoot := filepath.Join(d.Orch.Cfg.RunRoot, "worktrees")
	orphan := filepath.Join(wtRoot, "TST-9-1")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	// A worktree whose lease is alive must survive.
	live, err := d.Leases.Acquire("TST-8")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Release()
	keep := filepath.Join(wtRoot, "TST-8-1")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := d.gcWorktrees(context.Background()); err != nil {
		t.Fatalf("gcWorktrees: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Error("an orphaned worktree should be removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("a worktree with a live lease must not be removed")
	}
}

func TestSpendWindowRollsOff(t *testing.T) {
	var w spendWindow
	w.add(1.0)
	if got := w.total(time.Hour); got != 1.0 {
		t.Errorf("total = %v, want 1.0", got)
	}
	// An entry outside the window is forgotten.
	w.mu.Lock()
	w.entries[0].at = time.Now().Add(-2 * time.Hour)
	w.mu.Unlock()
	if got := w.total(time.Hour); got != 0 {
		t.Errorf("total = %v, want 0 after the entry aged out", got)
	}
}

func waitFor(t *testing.T, limit time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A ticket the orchestrator cannot decide must be reported once and then left
// alone, not re-attempted on every poll.
func TestDaemonSkipsUndecidableTicketsAfterOneReport(t *testing.T) {
	f := &fakeRaenil{
		criteria: []models.Criterion{{ID: "cr-1", Body: "someone looks at it", Kind: models.CriterionManual}},
		queue:    queued(),
	}
	d := newDaemon(t, f, fixRunner{})
	d.Cfg.PollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_ = d.Run(ctx)

	if !d.undecidable["TST-1"] {
		t.Error("an undecidable ticket should be remembered")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) != 0 {
		t.Errorf("an undecidable ticket must not be claimed or moved, states = %v", f.states)
	}
}

func TestDaemonRequireLabelFiltersTheQueue(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria(), queue: queued()} // queued issue has no labels
	d := newDaemon(t, f, fixRunner{})
	d.Cfg.PollInterval = 10 * time.Millisecond
	d.Cfg.RequireLabel = "ready-for-agent"

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = d.Run(ctx)

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) != 0 {
		t.Errorf("an unlabelled ticket must be left alone, states = %v", f.states)
	}
}

func TestHasLabel(t *testing.T) {
	is := models.Issue{Labels: []models.Label{{Name: "repo:web"}, {Name: "ready-for-agent"}}}
	if !hasLabel(is, "ready-for-agent") || !hasLabel(is, "READY-FOR-AGENT") {
		t.Error("label match should be case-insensitive")
	}
	if hasLabel(is, "needs-info") {
		t.Error("unexpected match")
	}
}

// blockingRunner holds the slot until released, so a second ticket is
// certain to be waiting behind the concurrency limit.
type blockingRunner struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

// Pointer receivers throughout: the struct holds a sync.Once, and a value
// receiver copies it — `go vet` calls that out, and it would silently give
// each call its own Once.
func (*blockingRunner) Name() string                           { return "blocking" }
func (*blockingRunner) Ready(context.Context, string) error    { return nil }
func (*blockingRunner) Health(context.Context) (string, error) { return "ok", nil }

func (r *blockingRunner) Run(ctx context.Context, _ RunRequest) (RunResult, error) {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
	case <-ctx.Done():
	}
	return RunResult{}, nil
}

// The daemon never claims more tickets than it can run.
//
// It used to acquire the lease and THEN wait for a concurrency slot, so
// --concurrency N claimed N+1 tickets: acquisition was never the thing being
// limited. Observed live as three leases taken in the same microsecond under
// --concurrency 2.
//
// The surplus lease is the damaging part, because KeepAlive only starts
// inside the worker goroutine. A ticket queued behind a busy slot therefore
// held a lease whose heartbeat stayed frozen at the moment it was taken:
// `status` reports it as ever more stale, indistinguishable from a crashed
// worker, and `stale()` judges ANOTHER host's lease on the heartbeat alone —
// so past the TTL a second machine would reclaim a ticket this one is about
// to work.
//
// Asserted as a count rather than as a heartbeat age on purpose: the beat
// interval is 30s, so a lease that has only just been taken legitimately has
// Heartbeat == Started, and a timing assertion here would be measuring the
// clock rather than the invariant.
func TestTheDaemonNeverClaimsMoreTicketsThanItCanRun(t *testing.T) {
	r := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	f := &fakeRaenil{criteria: passingCriteria(), queue: []models.Issue{
		{ID: "iss-1", Key: "TST-1", Title: "first"},
		{ID: "iss-2", Key: "TST-2", Title: "second"},
		{ID: "iss-3", Key: "TST-3", Title: "third"},
	}}
	d := newDaemon(t, f, r)
	d.Leases.TTL = time.Hour // nothing is reclaimed mid-test

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	<-r.started // one ticket is running and cannot finish

	// Hold it there and watch: the surplus claim appeared immediately, so a
	// short window is enough and a longer one only slows the suite.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		held, err := d.Leases.Held()
		if err != nil {
			t.Fatal(err)
		}
		if len(held) > d.Cfg.MaxConcurrent {
			t.Fatalf("claimed %d tickets with MaxConcurrent=%d: %v",
				len(held), d.Cfg.MaxConcurrent, held)
		}
		time.Sleep(20 * time.Millisecond)
	}

	close(r.release)
	cancel()
	<-done
}
