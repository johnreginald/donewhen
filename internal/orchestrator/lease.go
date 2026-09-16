package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ErrLeased means someone else holds the ticket.
var ErrLeased = errors.New("ticket is already being worked")

// Lease is a claim on one ticket, held as a file so an interactive
// `orchestrator run` and a daemon cannot pick up the same ticket.
type Lease struct {
	Ticket    string    `json:"ticket"`
	PID       int       `json:"pid"`
	Host      string    `json:"host"`
	Started   time.Time `json:"started"`
	Heartbeat time.Time `json:"heartbeat"`

	path string
}

// LeaseManager hands out leases from a directory.
type LeaseManager struct {
	Dir string
	// TTL is how old a heartbeat may be before the holder is presumed dead.
	TTL time.Duration
}

// DefaultLeaseTTL is deliberately several heartbeats long: a machine under load
// must not have its own live work stolen.
const DefaultLeaseTTL = 3 * time.Minute

// HeartbeatInterval is how often a holder should refresh its lease.
const HeartbeatInterval = 30 * time.Second

func (m *LeaseManager) ttl() time.Duration {
	if m.TTL > 0 {
		return m.TTL
	}
	return DefaultLeaseTTL
}

func (m *LeaseManager) path(ticket string) string {
	return filepath.Join(m.Dir, strings.ReplaceAll(ticket, "/", "_")+".lease")
}

// Acquire claims a ticket, or returns ErrLeased if a live holder has it.
//
// A lease whose heartbeat has gone stale AND whose process is gone is reclaimed:
// that is the crash-recovery path, and it is the only way a killed daemon's
// tickets ever become workable again.
func (m *LeaseManager) Acquire(ticket string) (*Lease, error) {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return nil, err
	}
	p := m.path(ticket)

	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			now := time.Now().UTC()
			host, _ := os.Hostname()
			l := &Lease{Ticket: ticket, PID: os.Getpid(), Host: host,
				Started: now, Heartbeat: now, path: p}
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			werr := enc.Encode(l)
			f.Close()
			if werr != nil {
				os.Remove(p)
				return nil, werr
			}
			return l, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		existing, rerr := readLease(p)
		if rerr != nil {
			// An unreadable lease is not a claim anyone can honour.
			os.Remove(p)
			continue
		}
		if !m.stale(existing) {
			return nil, fmt.Errorf("%w: held by pid %d on %s since %s",
				ErrLeased, existing.PID, existing.Host, existing.Started.Format(time.RFC3339))
		}
		os.Remove(p)
	}
	return nil, fmt.Errorf("%w: lease contended", ErrLeased)
}

// stale reports whether a lease may be taken from its holder. Both conditions
// must hold: the heartbeat has lapsed AND the process is gone. Either alone is
// a good way to steal live work.
func (m *LeaseManager) stale(l *Lease) bool {
	if time.Since(l.Heartbeat) < m.ttl() {
		return false
	}
	host, _ := os.Hostname()
	if l.Host != "" && l.Host != host {
		// Another machine's lease: the heartbeat is all we can judge by.
		return true
	}
	return !processAlive(l.PID)
}

// Beat refreshes the lease. A holder that stops beating will eventually be
// reclaimed.
func (l *Lease) Beat() error {
	l.Heartbeat = time.Now().UTC()
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// Release gives the ticket up.
func (l *Lease) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// KeepAlive beats until stop is closed. It is the caller's job to Release.
func (l *Lease) KeepAlive(stop <-chan struct{}) {
	t := time.NewTicker(HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = l.Beat()
		}
	}
}

// Held lists every lease currently on disk, live or stale.
func (m *LeaseManager) Held() ([]*Lease, error) {
	entries, err := os.ReadDir(m.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Lease
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".lease" {
			continue
		}
		l, err := readLease(filepath.Join(m.Dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// ReclaimStale removes leases whose holder is gone and reports which tickets
// were freed, so a starting daemon can reset them on the tracker.
func (m *LeaseManager) ReclaimStale() ([]string, error) {
	held, err := m.Held()
	if err != nil {
		return nil, err
	}
	var freed []string
	for _, l := range held {
		if !m.stale(l) {
			continue
		}
		if err := os.Remove(l.path); err == nil {
			freed = append(freed, l.Ticket)
		}
	}
	return freed, nil
}

func readLease(p string) (*Lease, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var l Lease
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if l.Ticket == "" {
		return nil, fmt.Errorf("lease %s has no ticket", p)
	}
	l.path = p
	return &l, nil
}

// processAlive reports whether a pid is still running. Signal 0 performs the
// permission and existence checks without delivering anything.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means it exists but belongs to someone else.
	return errors.Is(err, syscall.EPERM)
}
