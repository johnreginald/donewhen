// Package events is a tiny in-process publish/subscribe bus. Issue mutations
// publish a single domain event that fans out to SSE clients and Web Push.
package events

import (
	"sync"
	"time"

	"github.com/johnreginald/donewhen/internal/models"
)

// Event types.
const (
	IssueCreated      = "issue.created"
	IssueUpdated      = "issue.updated"
	IssueStateChanged = "issue.state_changed"
	IssueDeleted      = "issue.deleted"
	CommentAdded      = "comment.added"
	DocumentSaved     = "document.saved"
	DocumentDeleted   = "document.deleted"
	// MemberRemoved is published when a user loses membership of a workspace.
	// The SSE handler closes that user's open streams for the workspace and does
	// not forward the event to any client.
	MemberRemoved = "member.removed"
	// Resync is never published on the bus. The SSE handler writes it to a
	// client whose subscriber dropped events, as `event: resync` with data
	// `{}`. The client should refetch what it shows instead of trusting the
	// stream.
	Resync = "resync"
)

type Event struct {
	Type string `json:"type"`
	// WorkspaceID is the tenancy boundary for delivery: a subscriber only ever
	// receives events for the workspace it subscribed with.
	WorkspaceID string                `json:"workspaceId"`
	Actor       string                `json:"actor"` // human | ai
	Issue       *models.Issue         `json:"issue,omitempty"`
	IssueID     string                `json:"issueId,omitempty"`
	From        *models.WorkflowState `json:"from,omitempty"`
	To          *models.WorkflowState `json:"to,omitempty"`
	Comment     *models.Comment       `json:"comment,omitempty"`
	Document    *models.Document      `json:"document,omitempty"`
	DocumentID  string                `json:"documentId,omitempty"`
	// UserID names the subject of member.removed.
	UserID string    `json:"userId,omitempty"`
	At     time.Time `json:"at"`
}

type subscriber struct {
	ch     chan Event
	resync chan struct{} // cap 1: a pending signal means events were dropped
	wsID   string
	all    bool
}

// Subscription is one consumer's view of the bus.
type Subscription struct {
	// Events carries events for the subscribed workspace.
	Events <-chan Event
	// Resync receives a signal (at most one pending) after the subscriber's
	// buffer overflowed and an event was dropped. Reading it clears the flag.
	Resync <-chan struct{}
	// Close unsubscribes and closes Events. Safe to call more than once.
	Close func()
}

// Bus fans out events to all current subscribers. Sends are non-blocking:
// a subscriber whose buffer is full drops the event rather than stalling, and
// is told through Resync so it can recover.
type Bus struct {
	mu   sync.RWMutex
	subs map[*subscriber]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: make(map[*subscriber]struct{})}
}

// Subscribe returns a subscription scoped to one workspace. The empty id is not
// a wildcard: it matches no event, so a caller that failed to resolve a
// workspace gets nothing instead of everything. Scoping here rather than at the
// SSE layer means a leak cannot be reintroduced by a new consumer of the bus.
func (b *Bus) Subscribe(wsID string) Subscription {
	return b.subscribe(&subscriber{wsID: wsID})
}

// SubscribeAll returns a subscription to every workspace's events. It is for
// the one in-process consumer that fans out per event itself (Web Push); never
// hand it to a per-user stream.
func (b *Bus) SubscribeAll() Subscription {
	return b.subscribe(&subscriber{all: true})
}

func (b *Bus) subscribe(s *subscriber) Subscription {
	s.ch = make(chan Event, 32)
	s.resync = make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return Subscription{Events: s.ch, Resync: s.resync, Close: func() {
		b.mu.Lock()
		if _, ok := b.subs[s]; ok {
			delete(b.subs, s)
			close(s.ch)
		}
		b.mu.Unlock()
	}}
}

// Publish delivers e to every matching subscriber (best-effort, non-blocking).
func (b *Bus) Publish(e Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		if !s.all && (s.wsID == "" || s.wsID != e.WorkspaceID) {
			continue // different workspace: not this subscriber's business
		}
		select {
		case s.ch <- e:
		default: // slow consumer: drop the event and flag it once
			select {
			case s.resync <- struct{}{}:
			default:
			}
		}
	}
}
