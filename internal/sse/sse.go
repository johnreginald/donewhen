// Package sse streams domain events to connected browsers over
// text/event-stream (Server-Sent Events).
package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/events"
)

// PingInterval is how often an open stream writes a keep-alive comment and
// re-checks that its caller may still read the workspace.
const PingInterval = 25 * time.Second

// Revalidator reports whether the request's credential is still live and its
// user is still a member of the workspace.
type Revalidator func(r *http.Request, wsID, userID string) bool

type Handler struct {
	bus        *events.Bus
	revalidate Revalidator
	// PingInterval defaults to the package constant; tests shorten it.
	PingInterval time.Duration
	seq          atomic.Uint64
}

// NewHandler builds the stream handler. revalidate may be nil in tests that do
// not exercise revocation.
func NewHandler(bus *events.Bus, revalidate Revalidator) *Handler {
	return &Handler{bus: bus, revalidate: revalidate, PingInterval: PingInterval}
}

// ServeHTTP holds the connection open and writes each event as it arrives.
//
// The stream carries exactly one workspace, and only while the caller stays a
// member of it: it closes when the membership is removed (member.removed) or
// when a ping finds the session, token or membership gone. If the subscriber
// drops events because it reads too slowly, the client gets one
// `event: resync` with data `{}`.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// The stream is bound to the workspace that was active when it opened; the
	// client reopens it on switch. No workspace means no stream: there is no
	// "all workspaces" subscription for a browser.
	ws, ok := auth.WorkspaceFrom(r.Context())
	if !ok || ws.ID == "" {
		http.Error(w, `{"error":"no workspace resolved for this stream"}`, http.StatusForbidden)
		return
	}
	user, _ := auth.UserFrom(r.Context())

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering

	sub := h.bus.Subscribe(ws.ID)
	defer sub.Close()

	// Initial comment so the client's onopen fires promptly.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	every := h.PingInterval
	if every <= 0 {
		every = PingInterval
	}
	ping := time.NewTicker(every)
	defer ping.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if h.revalidate != nil && !h.revalidate(r, ws.ID, user.ID) {
				return
			}
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-sub.Resync:
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: {}\n\n", h.seq.Add(1), events.Resync)
			flusher.Flush()
		case e, ok := <-sub.Events:
			if !ok {
				return
			}
			if e.Type == events.MemberRemoved {
				if e.UserID == user.ID {
					return // our membership is gone: close at once
				}
				continue // about someone else; not for this client
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", h.seq.Add(1), e.Type, data)
			flusher.Flush()
		}
	}
}
