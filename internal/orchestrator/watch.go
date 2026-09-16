package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"raenil/internal/models"
)

// IssueEvent is one change streamed from Raenil's /api/events.
type IssueEvent struct {
	Type string `json:"type"`
	// Actor is "human" or "ai". It cannot distinguish one agent from another,
	// so it is informational only — the lease is what prevents double work.
	Actor string                `json:"actor"`
	Issue *models.Issue         `json:"issue,omitempty"`
	From  *models.WorkflowState `json:"from,omitempty"`
	To    *models.WorkflowState `json:"to,omitempty"`
	At    time.Time             `json:"at"`
}

// EnteredState reports whether this event moved an issue into the named state.
func (e IssueEvent) EnteredState(name string) bool {
	return e.Type == "issue.state_changed" && e.To != nil && strings.EqualFold(e.To.Name, name)
}

// Key returns the issue key the event concerns.
func (e IssueEvent) Key() string {
	if e.Issue == nil {
		return ""
	}
	return e.Issue.Key
}

// WatchIssues streams issue events until the context ends, reconnecting with
// backoff whenever the stream drops.
//
// This is a latency optimisation, never a delivery guarantee. Raenil's bus
// drops events for a slow consumer by design, and a dropped connection loses
// whatever happened while it was down — so anything relying on this must also
// reconcile periodically against the tracker's actual state.
func (c *RaenilClient) WatchIssues(ctx context.Context, onEvent func(IssueEvent)) error {
	backoff := time.Second
	for {
		err := c.streamOnce(ctx, onEvent)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			// A failed stream is normal operation, not a fatal condition: the
			// server restarts, the tailnet blips, a proxy times the stream out.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (c *RaenilClient) streamOnce(ctx context.Context, onEvent func(IssueEvent)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	if c.Workspace != "" {
		req.Header.Set("X-Workspace", c.Workspace)
	}

	// The stream is long-lived, so it cannot use the shared client's timeout.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("raenil events: %s", resp.Status)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			// Blank line ends an event.
			if data.Len() > 0 {
				var ev IssueEvent
				if json.Unmarshal([]byte(data.String()), &ev) == nil {
					onEvent(ev)
				}
				data.Reset()
			}
		case strings.HasPrefix(line, ":"):
			// Comment: ": connected", ": ping". Liveness only.
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("event stream closed")
}
