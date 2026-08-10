// Package events is a tiny in-process publish/subscribe bus. Issue mutations
// publish a single domain event that fans out to SSE clients and Web Push.
package events

import (
	"sync"
	"time"

	"raenil/internal/models"
)

// Event types.
const (
	IssueCreated      = "issue.created"
	IssueUpdated      = "issue.updated"
	IssueStateChanged = "issue.state_changed"
	IssueDeleted      = "issue.deleted"
	CommentAdded      = "comment.added"
)

type Event struct {
	Type string `json:"type"`
	// WorkspaceID is the tenancy boundary for delivery: a subscriber only ever
	// receives events for the workspace it subscribed with.
	WorkspaceID string                `json:"workspaceId"`
	Actor       string                `json:"actor"` // human | ai
	Issue   *models.Issue         `json:"issue,omitempty"`
	IssueID string                `json:"issueId,omitempty"`
	From    *models.WorkflowState `json:"from,omitempty"`
	To      *models.WorkflowState `json:"to,omitempty"`
	Comment *models.Comment       `json:"comment,omitempty"`
	At      time.Time             `json:"at"`
}

type subscriber struct {
	ch   chan Event
	wsID string
}

// Bus fans out events to all current subscribers. Sends are non-blocking:
// a subscriber whose buffer is full drops the event rather than stalling.
type Bus struct {
	mu   sync.RWMutex
	subs map[*subscriber]struct{}
}

func NewBus() *Bus {
	return &Bus{subs: make(map[*subscriber]struct{})}
}

// Subscribe returns a receive channel scoped to one workspace, and an
// unsubscribe func. Scoping here rather than at the SSE layer means a leak
// cannot be reintroduced by a new consumer of the bus.
func (b *Bus) Subscribe(wsID string) (<-chan Event, func()) {
	s := &subscriber{ch: make(chan Event, 32), wsID: wsID}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s.ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[s]; ok {
			delete(b.subs, s)
			close(s.ch)
		}
		b.mu.Unlock()
	}
}

// Publish delivers e to every subscriber (best-effort, non-blocking).
func (b *Bus) Publish(e Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		if s.wsID != "" && s.wsID != e.WorkspaceID {
			continue // different workspace: not this subscriber's business
		}
		select {
		case s.ch <- e:
		default: // slow consumer: drop
		}
	}
}
