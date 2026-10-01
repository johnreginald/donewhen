package api

import (
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/push"
)

// browserSubscription matches the JSON produced by PushSubscription.toJSON().
// ExpirationTime is declared but unused: the decoder disallows unknown fields,
// and the browser always includes "expirationTime" (usually null), so omitting
// it made every subscribe 400 with "invalid subscription".
type browserSubscription struct {
	Endpoint       string `json:"endpoint"`
	ExpirationTime any    `json:"expirationTime"`
	Keys           struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var sub browserSubscription
	if err := readJSON(r, &sub); err != nil || sub.Endpoint == "" {
		writeErr(w, http.StatusBadRequest, "invalid subscription")
		return
	}
	// The server will POST to this URL later, so it must be a public https
	// host — never an internal address.
	if err := push.ValidateEndpoint(r.Context(), sub.Endpoint, s.pushResolve); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Re-subscribing an endpoint rebinds it to the caller: the endpoint is a
	// capability held by one browser, and that browser is now this user's.
	err := s.store.SavePushSubscription(r.Context(), u.ID, models.PushSubscription{
		Endpoint: sub.Endpoint,
		P256dh:   sub.Keys.P256dh,
		Auth:     sub.Keys.Auth,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "subscribed"})
}

// handlePushUnsubscribe removes the caller's own subscription. Another user's
// endpoint is a 404 and stays put.
func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := readJSON(r, &body); err != nil || body.Endpoint == "" {
		writeErr(w, http.StatusBadRequest, "endpoint required")
		return
	}
	if handleStoreErr(w, s.store.DeletePushSubscription(r.Context(), u.ID, body.Endpoint)) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "unsubscribed"})
}
