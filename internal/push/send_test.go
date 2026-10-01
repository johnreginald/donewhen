package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/models"
)

// subKeys returns a valid (p256dh, auth) pair so webpush-go can encrypt.
func subKeys(t *testing.T) (string, string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(authSecret)
}

func testNotifier(t *testing.T, drop func(context.Context, string) error) *Notifier {
	t.Helper()
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	return &Notifier{
		cfg:         config.Config{VAPIDPublic: pub, VAPIDPrivate: priv, VAPIDSubject: "mailto:test@example.test"},
		client:      &http.Client{}, // the fake server is on loopback; the safe client is tested separately
		sendTimeout: 2 * time.Second,
		jobs:        make(chan job, queueSize),
		drop:        drop,
	}
}

func TestHungEndpointDoesNotBlockOthers(t *testing.T) {
	var got atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hang" {
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		got.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	defer close(release)

	n := testNotifier(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n.startWorkers(ctx)

	p256, authKey := subKeys(t)
	mk := func(path string) models.PushSubscription {
		return models.PushSubscription{Endpoint: srv.URL + path, P256dh: p256, Auth: authKey}
	}
	subs := []models.PushSubscription{mk("/hang")}
	for i := 0; i < 5; i++ {
		subs = append(subs, mk(fmt.Sprintf("/ok%d", i)))
	}

	start := time.Now()
	n.enqueue(subs, []byte(`{"title":"t"}`))
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Fatalf("enqueue blocked for %v", took)
	}
	deadline := time.After(time.Second)
	for got.Load() < 5 {
		select {
		case <-deadline:
			t.Fatalf("only %d of 5 healthy subscribers notified within 1s while one endpoint hangs", got.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSendIsBoundedByTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hang until the test ends
	}))
	defer srv.Close()
	defer close(release)
	n := testNotifier(t, nil)
	n.sendTimeout = 300 * time.Millisecond
	p256, authKey := subKeys(t)

	start := time.Now()
	n.sendOne(context.Background(), job{sub: models.PushSubscription{Endpoint: srv.URL, P256dh: p256, Auth: authKey}, body: []byte("{}")})
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("send was not bounded by the timeout: %v", took)
	}
}

func TestRetiredSubscriptionIsDeleted(t *testing.T) {
	for _, code := range []int{http.StatusGone, http.StatusNotFound, http.StatusCreated, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		var mu sync.Mutex
		var dropped []string
		n := testNotifier(t, func(_ context.Context, ep string) error {
			mu.Lock()
			defer mu.Unlock()
			dropped = append(dropped, ep)
			return nil
		})
		p256, authKey := subKeys(t)
		n.sendOne(context.Background(), job{sub: models.PushSubscription{Endpoint: srv.URL, P256dh: p256, Auth: authKey}, body: []byte("{}")})
		srv.Close()
		want := code == http.StatusGone || code == http.StatusNotFound
		if got := len(dropped) == 1; got != want {
			t.Errorf("status %d: dropped=%v, want dropped=%v", code, dropped, want)
		}
	}
}

func TestEnqueueDropsWhenQueueFull(t *testing.T) {
	n := testNotifier(t, nil)
	n.jobs = make(chan job, 1) // no workers: the queue fills
	done := make(chan struct{})
	go func() {
		n.enqueue(make([]models.PushSubscription, 10), []byte("{}"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked on a full queue")
	}
}
