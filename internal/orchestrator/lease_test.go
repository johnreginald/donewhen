package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newLeases(t *testing.T) *LeaseManager {
	t.Helper()
	return &LeaseManager{Dir: t.TempDir(), TTL: 50 * time.Millisecond}
}

func TestLeaseExcludesASecondHolder(t *testing.T) {
	m := newLeases(t)
	l, err := m.Acquire("TST-1")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := m.Acquire("TST-1"); !errors.Is(err, ErrLeased) {
		t.Fatalf("second Acquire err = %v, want ErrLeased", err)
	}
	// A different ticket is unaffected.
	if _, err := m.Acquire("TST-2"); err != nil {
		t.Errorf("unrelated ticket should be acquirable: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := m.Acquire("TST-1"); err != nil {
		t.Errorf("after release, Acquire should succeed: %v", err)
	}
}

// The important half of crash recovery: a dead holder's lease is reclaimable.
func TestLeaseReclaimedWhenHolderIsGone(t *testing.T) {
	m := newLeases(t)
	writeStaleLease(t, m, "TST-1", 999999, time.Now().Add(-time.Hour))

	l, err := m.Acquire("TST-1")
	if err != nil {
		t.Fatalf("a dead holder's lease should be reclaimable: %v", err)
	}
	if l.PID != os.Getpid() {
		t.Errorf("reclaimed lease should belong to us, pid = %d", l.PID)
	}
}

// The other half, and the more dangerous one: live work must never be stolen,
// even when the heartbeat has lapsed because the machine was busy.
func TestLeaseNotStolenFromLiveProcess(t *testing.T) {
	m := newLeases(t)
	writeStaleLease(t, m, "TST-1", os.Getpid(), time.Now().Add(-time.Hour))

	if _, err := m.Acquire("TST-1"); !errors.Is(err, ErrLeased) {
		t.Fatalf("a lapsed heartbeat from a LIVE process must not be reclaimed, got %v", err)
	}
	freed, err := m.ReclaimStale()
	if err != nil {
		t.Fatal(err)
	}
	if len(freed) != 0 {
		t.Errorf("ReclaimStale freed a live holder's ticket: %v", freed)
	}
}

func TestReclaimStaleReportsFreedTickets(t *testing.T) {
	m := newLeases(t)
	writeStaleLease(t, m, "TST-1", 999999, time.Now().Add(-time.Hour))
	writeStaleLease(t, m, "TST-2", 999998, time.Now().Add(-time.Hour))
	if _, err := m.Acquire("TST-3"); err != nil { // live, ours
		t.Fatal(err)
	}

	freed, err := m.ReclaimStale()
	if err != nil {
		t.Fatal(err)
	}
	if len(freed) != 2 {
		t.Errorf("freed = %v, want the two dead tickets", freed)
	}
	held, _ := m.Held()
	if len(held) != 1 || held[0].Ticket != "TST-3" {
		t.Errorf("our own live lease should survive, held = %v", held)
	}
}

func TestBeatRefreshesTheHeartbeat(t *testing.T) {
	m := newLeases(t)
	l, err := m.Acquire("TST-1")
	if err != nil {
		t.Fatal(err)
	}
	before := l.Heartbeat
	time.Sleep(5 * time.Millisecond)
	if err := l.Beat(); err != nil {
		t.Fatal(err)
	}
	held, _ := m.Held()
	if len(held) != 1 {
		t.Fatalf("expected one lease, got %d", len(held))
	}
	if !held[0].Heartbeat.After(before) {
		t.Error("Beat should move the heartbeat forward on disk")
	}
}

func TestUnreadableLeaseIsNotAClaim(t *testing.T) {
	m := newLeases(t)
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "TST-1.lease"), []byte("{garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire("TST-1"); err != nil {
		t.Errorf("a corrupt lease file should not block work forever: %v", err)
	}
}

func writeStaleLease(t *testing.T, m *LeaseManager, ticket string, pid int, beat time.Time) {
	t.Helper()
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	host, _ := os.Hostname()
	b, _ := json.MarshalIndent(Lease{
		Ticket: ticket, PID: pid, Host: host,
		Started: beat, Heartbeat: beat,
	}, "", "  ")
	if err := os.WriteFile(m.path(ticket), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The hand-run path is the common one, so it must claim its ticket too.
// Two terminals on the same ticket would otherwise each cut a worktree and a
// branch and race to commit.
func TestWorkClaimsALease(t *testing.T) {
	f := &fakeRaenil{criteria: passingCriteria()}
	o := newOrch(t, f, fixRunner{})
	o.Cfg.Model = "cheap/model"
	o.Leases = &LeaseManager{Dir: filepath.Join(t.TempDir(), "leases"), TTL: time.Minute}

	// Someone else already holds it.
	held, err := o.Leases.Acquire("TST-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: TriagePolicy{MaxAttempts: 1}}); !errors.Is(err, ErrLeased) {
		t.Fatalf("Work should refuse a ticket someone else holds, got %v", err)
	}
	f.mu.Lock()
	states := len(f.states)
	f.mu.Unlock()
	if states != 0 {
		t.Error("a refused ticket must not be touched at all")
	}

	// Once released, it works and gives the lease back.
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Work(context.Background(), "TST-1", WorkConfig{Triage: TriagePolicy{MaxAttempts: 1}}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	after, _ := o.Leases.Held()
	if len(after) != 0 {
		t.Errorf("the lease should be released when Work returns, still held: %v", after)
	}
}
