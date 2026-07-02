package api

import (
	"net/http"

	"raenil/internal/auth"
	"raenil/internal/models"
)

// browserSubscription matches the JSON produced by PushManager.subscribe().
type browserSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
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

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := readJSON(r, &body); err != nil || body.Endpoint == "" {
		writeErr(w, http.StatusBadRequest, "endpoint required")
		return
	}
	if handleStoreErr(w, s.store.DeletePushSubscriptionByEndpoint(r.Context(), body.Endpoint)) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "unsubscribed"})
}
