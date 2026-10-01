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
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

const (
	// workerCount bounds concurrent sends; sendTimeout bounds each one, so a
	// hung endpoint can hold a worker for at most this long.
	workerCount = 4
	sendTimeout = 10 * time.Second
	queueSize   = 512
)

type Notifier struct {
	store *store.Store
	bus   *events.Bus
	cfg   config.Config

	client      webpush.HTTPClient // refuses non-public addresses; tests swap it
	sendTimeout time.Duration
	jobs        chan job
	// drop removes a subscription the push service reported gone (404/410).
	drop func(ctx context.Context, endpoint string) error
}

// job is one notification for one device.
type job struct {
	sub  models.PushSubscription
	body []byte
}

func NewNotifier(s *store.Store, bus *events.Bus, cfg config.Config) *Notifier {
	return &Notifier{
		store: s, bus: bus, cfg: cfg,
		client:      NewSafeClient(),
		sendTimeout: sendTimeout,
		jobs:        make(chan job, queueSize),
		drop:        s.DeletePushSubscriptionByEndpoint,
	}
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
	n.startWorkers(ctx)
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

// broadcast queues a notification for the devices of the workspace's members —
// a phone whose owner cannot open the issue must not buzz for it. It only
// enqueues: the sends happen on the worker pool, so the bus loop never waits
// on a push service.
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
	n.enqueue(subs, body)
}

// enqueue hands each subscription to the worker pool without blocking. If the
// queue is full (workers are all stuck on slow endpoints) the notification is
// dropped and logged rather than stalling the event loop.
func (n *Notifier) enqueue(subs []models.PushSubscription, body []byte) {
	for _, s := range subs {
		select {
		case n.jobs <- job{sub: s, body: body}:
		default:
			log.Printf("push: queue full, dropping notification for %s", endpointHost(s.Endpoint))
		}
	}
}

// startWorkers launches the fixed pool that performs sends; it stops with ctx.
func (n *Notifier) startWorkers(ctx context.Context) {
	for i := 0; i < workerCount; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-n.jobs:
					n.sendOne(ctx, j)
				}
			}
		}()
	}
}

// sendOne delivers one notification, bounded by the per-send timeout.
func (n *Notifier) sendOne(ctx context.Context, j job) {
	ctx, cancel := context.WithTimeout(ctx, n.sendTimeout)
	defer cancel()
	sub := &webpush.Subscription{
		Endpoint: j.sub.Endpoint,
		Keys:     webpush.Keys{P256dh: j.sub.P256dh, Auth: j.sub.Auth},
	}
	resp, err := webpush.SendNotificationWithContext(ctx, j.body, sub, &webpush.Options{
		HTTPClient:      n.client,
		Subscriber:      n.cfg.VAPIDSubject,
		VAPIDPublicKey:  n.cfg.VAPIDPublic,
		VAPIDPrivateKey: n.cfg.VAPIDPrivate,
		TTL:             30,
	})
	if err != nil {
		log.Printf("push: send to %s: %v", endpointHost(j.sub.Endpoint), err)
		return
	}
	defer resp.Body.Close()
	// A non-2xx from the push service is silent otherwise (webpush-go only
	// returns err on transport failure) — log status + body so a rejected
	// push (bad VAPID, expired, quota) is diagnosable.
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("push: %d from %s: %s", resp.StatusCode, endpointHost(j.sub.Endpoint), strings.TrimSpace(string(b)))
	}
	// Drop subscriptions the push service has retired.
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		if err := n.drop(ctx, j.sub.Endpoint); err != nil {
			log.Printf("push: drop retired subscription: %v", err)
		}
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
