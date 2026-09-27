package api

import (
	"net/http"
	"strconv"

	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

// handleListReviews is a ticket's agent reviews, newest first.
func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	out, err := s.store.ListReviews(r.Context(), ws(r), id)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(out))
}

// handleSaveReview records a review the runner host made.
func (s *Server) handleSaveReview(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	var body models.Review
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	body.IssueID = id
	rv, err := s.store.SaveReview(r.Context(), ws(r), body)
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "review.saved", IssueID: id})
	writeJSON(w, http.StatusCreated, rv)
}

// handleFlow is the factory's flow numbers over ?days= (default 7).
func (s *Server) handleFlow(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	f, err := s.store.FlowStats(r.Context(), ws(r), days)
	if handleStoreErr(w, err) {
		return
	}
	if f.Runners == nil {
		f.Runners = []store.RunnerFlow{}
	}
	writeJSON(w, 200, f)
}
