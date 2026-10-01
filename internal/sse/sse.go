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

type Handler struct {
	bus *events.Bus
	seq atomic.Uint64
}

func NewHandler(bus *events.Bus) *Handler {
	return &Handler{bus: bus}
}

// ServeHTTP holds the connection open and writes each event as it arrives.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering

	// The stream is bound to the workspace that was active when it opened; the
	// client reopens it on switch.
	var wsID string
	if ws, ok := auth.WorkspaceFrom(r.Context()); ok {
		wsID = ws.ID
	}
	ch, unsub := h.bus.Subscribe(wsID)
	defer unsub()

	// Initial comment so the client's onopen fires promptly.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			id := h.seq.Add(1)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, e.Type, data)
			flusher.Flush()
		}
	}
}
