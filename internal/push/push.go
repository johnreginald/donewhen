// Package push delivers Web Push (VAPID) notifications on domain events so a
// closed/locked phone still gets notified when a status changes.
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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
	ch, unsub := n.bus.Subscribe()
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if p, want := n.build(e); want {
				n.broadcast(ctx, p)
			}
		}
	}
}

// build turns an event into a notification payload, or want=false to skip.
func (n *Notifier) build(e events.Event) (payload, bool) {
	actor := "you"
	if e.Actor == "ai" {
		actor = "AI"
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

func (n *Notifier) broadcast(ctx context.Context, p payload) {
	body, err := json.Marshal(p)
	if err != nil {
		return
	}
	subs, err := n.store.ListPushSubscriptions(ctx)
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
		// Drop subscriptions the push service has retired.
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			_ = n.store.DeletePushSubscriptionByEndpoint(ctx, s.Endpoint)
		}
		resp.Body.Close()
	}
}

// GenerateVAPIDKeys returns a fresh (private, public) VAPID keypair.
func GenerateVAPIDKeys() (private, public string, err error) {
	return webpush.GenerateVAPIDKeys()
}
