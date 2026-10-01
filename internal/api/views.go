package api

import (
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/store"
)

// Saved views are private to their owner: every call is scoped to the signed-in
// user as well as the workspace, so another member's id answers 404.

func (s *Server) handleListViews(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	views, err := s.store.ListViews(r.Context(), ws(r), u.ID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, views)
}

type viewReq struct {
	Name     *string `json:"name"`
	Query    *string `json:"query"`
	Layout   *string `json:"layout"`
	Position *int    `json:"position"`
}

func (s *Server) handleCreateView(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req viewReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	layout := "list"
	if req.Layout != nil {
		layout = *req.Layout
	}
	v, err := s.store.CreateView(r.Context(), ws(r), u.ID, derefStr(req.Name), derefStr(req.Query), layout)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) handleUpdateView(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req viewReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	v, err := s.store.UpdateView(r.Context(), ws(r), u.ID, r.PathValue("id"), store.ViewPatch{
		Name: req.Name, Query: req.Query, Layout: req.Layout, Position: req.Position,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteView(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := s.store.DeleteView(r.Context(), ws(r), u.ID, r.PathValue("id")); handleStoreErr(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
