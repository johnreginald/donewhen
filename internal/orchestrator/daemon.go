package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"raenil/internal/models"
)

// DaemonConfig tunes unattended operation.
type DaemonConfig struct {
	// ReadyState is the queue the daemon pulls from.
	ReadyState string
	// PollInterval is how often the queue is checked when idle.
	PollInterval time.Duration
	// MaxConcurrent caps tickets in flight.
	MaxConcurrent int
	// MaxUSDPerHour halts the daemon once spend in a rolling hour exceeds this.
	// Zero disables the check, which is how people wake up to a large bill.
	MaxUSDPerHour float64
	// StopFile halts the daemon when it exists — a kill switch that needs no
	// signal, no port, and no access to the process.
	StopFile string
	// RequireLabel, when set, restricts the queue to issues carrying it. The
	// canonical value is "ready-for-agent": a ticket can be Ready for a human
	// without being work an orchestrator should take.
	RequireLabel string
	// Work configures each ticket's attempt loop.
	Work WorkConfig
}

func (c DaemonConfig) withDefaults(runRoot string) DaemonConfig {
	if c.ReadyState == "" {
		c.ReadyState = "Ready"
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 30 * time.Second
	}
	if c.MaxConcurrent <= 0 {
		// One. Two workers in the same repository produce conflicts neither can
		// resolve, and this daemon serves a single repository.
		c.MaxConcurrent = 1
	}
	if c.StopFile == "" {
		c.StopFile = filepath.Join(runRoot, "STOP")
	}
	return c
}

// Daemon works the Ready queue unattended.
type Daemon struct {
	Orch   *Orchestrator
	Leases *LeaseManager
	Cfg    DaemonConfig

	spend spendWindow
	// undecidable remembers tickets that failed pre-flight, so the daemon logs
	// the reason once instead of re-attempting them on every poll. A ticket the
	// orchestrator cannot decide is not necessarily a broken ticket — it may
	// simply be a human's to do — so it is skipped, not bounced.
	undecidable map[string]bool
	// repoLock serialises work on one repository. Concurrency across separate
	// repositories is safe; concurrency within one is not.
	repoLock sync.Mutex
}

// Run polls until the context is cancelled, the kill switch appears, or the
// hourly budget is spent.
func (d *Daemon) Run(ctx context.Context) error {
	cfg := d.Cfg.withDefaults(d.Orch.Cfg.RunRoot)
	if d.Leases == nil {
		d.Leases = &LeaseManager{Dir: filepath.Join(d.Orch.Cfg.RunRoot, "leases")}
	}

	if err := d.recover(ctx); err != nil {
		d.Orch.logf("warning: recovery incomplete: %v", err)
	}

	sem := make(chan struct{}, cfg.MaxConcurrent)
	var wg sync.WaitGroup
	defer wg.Wait()

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	d.Orch.logf("daemon up: queue=%q poll=%s concurrency=%d budget=$%.2f/h stop=%s",
		cfg.ReadyState, cfg.PollInterval, cfg.MaxConcurrent, cfg.MaxUSDPerHour, cfg.StopFile)

	for {
		if halt, reason := d.shouldHalt(cfg); halt {
			d.Orch.logf("halting: %s", reason)
			return nil
		}

		issues, err := d.Orch.Raenil.IssuesInState(ctx, cfg.ReadyState, 50)
		if err != nil {
			d.Orch.logf("queue read failed: %v", err)
		}
		for _, issue := range issues {
			if d.undecidable[issue.Key] {
				continue
			}
			if cfg.RequireLabel != "" && !hasLabel(issue, cfg.RequireLabel) {
				continue
			}
			if halt, reason := d.shouldHalt(cfg); halt {
				d.Orch.logf("halting: %s", reason)
				return nil
			}
			lease, err := d.Leases.Acquire(issue.Key)
			if errors.Is(err, ErrLeased) {
				continue
			}
			if err != nil {
				d.Orch.logf("lease %s failed: %v", issue.Key, err)
				continue
			}

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				_ = lease.Release()
				return ctx.Err()
			}

			wg.Add(1)
			go func(key string, l *Lease) {
				defer wg.Done()
				defer func() { <-sem }()
				defer l.Release()

				stop := make(chan struct{})
				defer close(stop)
				go l.KeepAlive(stop)

				d.workOne(ctx, key, cfg)
			}(issue.Key, lease)
		}

		select {
		case <-ctx.Done():
			d.Orch.logf("shutting down, releasing leases")
			return nil
		case <-ticker.C:
		}
	}
}

// workOne runs a single ticket to completion, serialised per repository.
func (d *Daemon) workOne(ctx context.Context, key string, cfg DaemonConfig) {
	d.repoLock.Lock()
	defer d.repoLock.Unlock()

	// Re-check the kill switch: a ticket may have waited behind another.
	if halt, reason := d.shouldHalt(cfg); halt {
		d.Orch.logf("skipping %s: %s", key, reason)
		return
	}

	d.Orch.logf("--- %s", key)
	v, err := d.Orch.Work(ctx, key, cfg.Work)
	if err != nil {
		d.Orch.logf("%s skipped: %v", key, err)
		// Pre-flight refusals are a property of the ticket, not a transient
		// failure, so stop reconsidering it until the daemon restarts.
		if d.undecidable == nil {
			d.undecidable = map[string]bool{}
		}
		d.undecidable[key] = true
		return
	}
	d.spend.add(v.CostUSD)
	d.Orch.logf("--- %s: %s ($%.4f, hour total $%.4f)", key, v.Status, v.CostUSD, d.spend.total(time.Hour))
}

// shouldHalt reports whether the daemon must stop, and why.
func (d *Daemon) shouldHalt(cfg DaemonConfig) (bool, string) {
	if cfg.StopFile != "" {
		if _, err := os.Stat(cfg.StopFile); err == nil {
			return true, "kill switch present at " + cfg.StopFile
		}
	}
	if cfg.MaxUSDPerHour > 0 {
		if spent := d.spend.total(time.Hour); spent >= cfg.MaxUSDPerHour {
			return true, fmt.Sprintf("hourly budget exhausted: $%.4f of $%.2f", spent, cfg.MaxUSDPerHour)
		}
	}
	return false, ""
}

// recover cleans up after a daemon that did not shut down cleanly.
//
// Tickets whose holder is gone are put back on the queue, and the worktrees they
// left behind are removed. Without this, a killed daemon's tickets stay In
// Progress forever and nothing picks them up again.
func (d *Daemon) recover(ctx context.Context) error {
	freed, err := d.Leases.ReclaimStale()
	if err != nil {
		return err
	}
	var problems []string
	for _, ticket := range freed {
		d.Orch.logf("reclaiming %s from a dead holder", ticket)
		issue, err := d.Orch.Raenil.Issue(ctx, ticket)
		if err != nil {
			problems = append(problems, ticket+": "+err.Error())
			continue
		}
		if err := d.Orch.Raenil.SetState(ctx, issue.ID, d.Cfg.withDefaults(d.Orch.Cfg.RunRoot).ReadyState); err != nil {
			problems = append(problems, ticket+": "+err.Error())
		}
		_ = d.Orch.Raenil.Comment(ctx, issue.ID,
			"A previous orchestrator run ended without finishing this ticket. It has been returned to the queue.")
	}

	if err := d.gcWorktrees(ctx); err != nil {
		problems = append(problems, "worktrees: "+err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("%v", problems)
	}
	return nil
}

// gcWorktrees removes worktree directories with no live lease behind them.
func (d *Daemon) gcWorktrees(ctx context.Context) error {
	root := filepath.Join(d.Orch.Cfg.RunRoot, "worktrees")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	held := map[string]bool{}
	if leases, err := d.Leases.Held(); err == nil {
		for _, l := range leases {
			held[l.Ticket] = true
		}
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Directories are "<TICKET>-<attempt>".
		name := e.Name()
		ticket := name
		if i := strings.LastIndexByte(name, '-'); i > 0 {
			ticket = name[:i]
		}
		if held[ticket] {
			continue
		}
		d.Orch.logf("removing orphaned worktree %s", name)
		_ = os.RemoveAll(filepath.Join(root, name))
	}
	// Tell git the directories are gone.
	_, _ = git(ctx, d.Orch.Cfg.Repo, "worktree", "prune")
	return nil
}

// spendWindow tracks spend over a rolling period so the budget guard reflects
// recent activity rather than the whole lifetime of the process.
type spendWindow struct {
	mu      sync.Mutex
	entries []spendEntry
}

type spendEntry struct {
	at  time.Time
	usd float64
}

func (w *spendWindow) add(usd float64) {
	if usd <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, spendEntry{at: time.Now(), usd: usd})
}

func (w *spendWindow) total(window time.Duration) float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := time.Now().Add(-window)
	var sum float64
	kept := w.entries[:0]
	for _, e := range w.entries {
		if e.at.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
		sum += e.usd
	}
	w.entries = kept
	return sum
}

// hasLabel reports whether an issue carries a label by name.
func hasLabel(issue models.Issue, name string) bool {
	for _, l := range issue.Labels {
		if strings.EqualFold(l.Name, name) {
			return true
		}
	}
	return false
}
