package api

import (
	"net/http"

	"kanri/internal/models"
)

// ---- states ----

func (s *Server) handleListStates(w http.ResponseWriter, r *http.Request) {
	states, err := s.store.ListStates(r.Context())
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(states))
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetState(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, st)
}

// ---- labels ----

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.store.ListLabels(r.Context())
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(labels))
}

func (s *Server) handleListLabelGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListLabelGroups(r.Context())
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(groups))
}

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Group string `json:"group"`
	}
	if err := readJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	l, err := s.store.CreateLabel(r.Context(), body.Name, body.Color, body.Group)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// ---- initiatives ----

func (s *Server) handleListInitiatives(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListInitiatives(r.Context())
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(items))
}

func (s *Server) handleGetInitiative(w http.ResponseWriter, r *http.Request) {
	i, err := s.store.GetInitiative(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, i)
}

func (s *Server) handleSaveInitiative(w http.ResponseWriter, r *http.Request) {
	var i models.Initiative
	if err := readJSON(r, &i); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if id := r.PathValue("id"); id != "" {
		i.ID = id
	}
	if i.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	saved, err := s.store.SaveInitiative(r.Context(), i)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleDeleteInitiative(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteInitiative(r.Context(), r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ---- projects ----

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProjects(r.Context(), r.URL.Query().Get("initiative"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(items))
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProject(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) handleSaveProject(w http.ResponseWriter, r *http.Request) {
	var p models.Project
	if err := readJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if id := r.PathValue("id"); id != "" {
		p.ID = id
	}
	if p.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	saved, err := s.store.SaveProject(r.Context(), p)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteProject(r.Context(), r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
