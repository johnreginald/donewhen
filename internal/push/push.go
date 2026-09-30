// Package push delivers Web Push (VAPID) notifications on domain events so a
// closed/locked phone still gets notified when a status changes.
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"

	"raenil/internal/config"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

type Notifier struct {
	store *store.Store
	bus   *events.Bus
	cfg   config.Config
}

func NewNotifier(s *store.Store, bus *events.Bus, cfg config.Config) *Notifier {
	return &Notifier{store: s, bus: bus, cfg: cfg}
}

type payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// Run subscribes to the bus and pushes notifications until ctx is canceled.
func (n *Notifier) Run(ctx context.Context) {
	if !n.cfg.PushEnabled() {
		log.Println("push: VAPID keys not set, Web Push disabled")
		return
	}
	// The empty workspace id means "every workspace": the notifier is the one
	// consumer that must see all events, because it fans out per event to that
	// event's members rather than holding a single workspace open.
	ch, unsub := n.bus.Subscribe("")
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if p, want := n.build(e, n.aiName(ctx, e.WorkspaceID)); want {
				n.broadcast(ctx, e.WorkspaceID, p)
			}
		}
	}
}

// aiName is what the workspace calls its AI actor; "Clanker" if unknown.
func (n *Notifier) aiName(ctx context.Context, wsID string) string {
	if wsID != "" {
		if w, err := n.store.GetWorkspace(ctx, wsID); err == nil && w.AIName != "" {
			return w.AIName
		}
	}
	return "Clanker"
}

// build turns an event into a notification payload, or want=false to skip.
// aiName labels AI actions.
func (n *Notifier) build(e events.Event, aiName string) (payload, bool) {
	actor := "you"
	if e.Actor == "ai" {
		actor = aiName
	}
	switch e.Type {
	case events.IssueStateChanged:
		if e.Issue == nil || e.To == nil {
			return payload{}, false
		}
		return payload{
			Title: fmt.Sprintf("%s → %s", e.Issue.Key, e.To.Name),
			Body:  fmt.Sprintf("%s (moved by %s)", e.Issue.Title, actor),
			URL:   n.issueURL(e.Issue),
			Tag:   e.Issue.ID,
		}, true
	case events.IssueCreated:
		if e.Issue == nil {
			return payload{}, false
		}
		return payload{
			Title: fmt.Sprintf("New: %s", e.Issue.Key),
			Body:  fmt.Sprintf("%s (created by %s)", e.Issue.Title, actor),
			URL:   n.issueURL(e.Issue),
			Tag:   e.Issue.ID,
		}, true
	case events.CommentAdded:
		if e.Issue == nil {
			return payload{}, false
		}
		return payload{
			Title: fmt.Sprintf("Comment on %s", e.Issue.Key),
			Body:  fmt.Sprintf("New comment by %s", actor),
			URL:   n.issueURL(e.Issue),
			Tag:   e.Issue.ID,
		}, true
	}
	return payload{}, false
}

func (n *Notifier) issueURL(is *models.Issue) string {
	return fmt.Sprintf("%s/issue/%s", n.cfg.BaseURL, is.Key)
}

// broadcast notifies only the devices of the workspace's members — a phone
// whose owner cannot open the issue must not buzz for it.
func (n *Notifier) broadcast(ctx context.Context, wsID string, p payload) {
	if wsID == "" {
		return // unattributed event: no membership to resolve, so nobody to tell
	}
	body, err := json.Marshal(p)
	if err != nil {
		return
	}
	subs, err := n.store.ListPushSubscriptions(ctx, wsID)
	if err != nil {
		log.Printf("push: list subscriptions: %v", err)
		return
	}
	for _, s := range subs {
		sub := &webpush.Subscription{
			Endpoint: s.Endpoint,
			Keys:     webpush.Keys{P256dh: s.P256dh, Auth: s.Auth},
		}
		resp, err := webpush.SendNotification(body, sub, &webpush.Options{
			Subscriber:      n.cfg.VAPIDSubject,
			VAPIDPublicKey:  n.cfg.VAPIDPublic,
			VAPIDPrivateKey: n.cfg.VAPIDPrivate,
			TTL:             30,
		})
		if err != nil {
			log.Printf("push: send: %v", err)
			continue
		}
		// A non-2xx from the push service is silent otherwise (webpush-go only
		// returns err on transport failure) — log status + body so a rejected
		// push (bad VAPID, expired, quota) is diagnosable.
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			log.Printf("push: %d from %s: %s", resp.StatusCode, endpointHost(s.Endpoint), strings.TrimSpace(string(b)))
		}
		// Drop subscriptions the push service has retired.
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			_ = n.store.DeletePushSubscriptionByEndpoint(ctx, s.Endpoint)
		}
		resp.Body.Close()
	}
}

// endpointHost returns just the host of a push endpoint for log lines
// (fcm.googleapis.com, updates.push.services.mozilla.com, …) without leaking
// the full per-device token.
func endpointHost(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return "?"
}

// GenerateVAPIDKeys returns a fresh (private, public) VAPID keypair.
func GenerateVAPIDKeys() (private, public string, err error) {
	return webpush.GenerateVAPIDKeys()
}
