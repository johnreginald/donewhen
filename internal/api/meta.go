package api

import (
	"encoding/json"
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

// ---- states ----

func (s *Server) handleListStates(w http.ResponseWriter, r *http.Request) {
	states, err := s.store.ListStates(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(states))
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetState(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, st)
}

// ---- labels ----

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.store.ListLabels(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(labels))
}

func (s *Server) handleListLabelGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListLabelGroups(r.Context(), ws(r))
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
	l, err := s.store.CreateLabel(r.Context(), ws(r), body.Name, body.Color, body.Group)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

// ---- initiatives ----

func (s *Server) handleListInitiatives(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListInitiatives(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(items))
}

func (s *Server) handleGetInitiative(w http.ResponseWriter, r *http.Request) {
	i, err := s.store.GetInitiative(r.Context(), ws(r), r.PathValue("id"))
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
	if i.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	saved, err := s.store.SaveInitiative(r.Context(), ws(r), i)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

// handleUpdateInitiative is PATCH: only the fields present in the body change.
func (s *Server) handleUpdateInitiative(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          *string         `json:"name"`
		DescriptionMD *string         `json:"descriptionMd"`
		Status        *string         `json:"status"`
		Position      *int            `json:"position"`
		RepoURL       json.RawMessage `json:"repoUrl"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil && *body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	repo, ok := patchString(body.RepoURL)
	if !ok {
		writeErr(w, http.StatusBadRequest, "repoUrl must be a string or null")
		return
	}
	saved, err := s.store.UpdateInitiative(r.Context(), ws(r), r.PathValue("id"), store.InitiativePatch{
		Name: body.Name, DescriptionMD: body.DescriptionMD, Status: body.Status,
		Position: body.Position, RepoURL: repo,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

// patchString reads a nullable string field of a PATCH body: absent -> nil
// (leave unchanged), null or "" -> "" (clear), a string -> that string. ok is
// false for any other JSON type.
func patchString(raw json.RawMessage) (*string, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if string(raw) == "null" {
		empty := ""
		return &empty, true
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	return &v, true
}

func (s *Server) handleDeleteInitiative(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteInitiative(r.Context(), ws(r), r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ---- projects ----

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListProjects(r.Context(), ws(r), r.URL.Query().Get("initiative"), r.URL.Query().Get("archived"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(items))
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProject(r.Context(), ws(r), r.PathValue("id"))
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
	if p.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	saved, err := s.store.SaveProject(r.Context(), ws(r), p)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

// handleUpdateProject is PATCH: only the fields present in the body change.
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          *string         `json:"name"`
		DescriptionMD *string         `json:"descriptionMd"`
		Status        *string         `json:"status"`
		Position      *int            `json:"position"`
		RepoURL       json.RawMessage `json:"repoUrl"`
		InitiativeID  json.RawMessage `json:"initiativeId"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil && *body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	repo, ok := patchString(body.RepoURL)
	if !ok {
		writeErr(w, http.StatusBadRequest, "repoUrl must be a string or null")
		return
	}
	ini, ok := patchString(body.InitiativeID)
	if !ok {
		writeErr(w, http.StatusBadRequest, "initiativeId must be a string or null")
		return
	}
	saved, err := s.store.UpdateProject(r.Context(), ws(r), r.PathValue("id"), store.ProjectPatch{
		Name: body.Name, DescriptionMD: body.DescriptionMD, Status: body.Status,
		Position: body.Position, RepoURL: repo, InitiativeID: ini,
	})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	s.setArchived(w, r, true)
}

func (s *Server) handleUnarchiveProject(w http.ResponseWriter, r *http.Request) {
	s.setArchived(w, r, false)
}

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	p, err := s.store.ArchiveProject(r.Context(), ws(r), r.PathValue("id"), archived, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteProject(r.Context(), ws(r), r.PathValue("id"))) {
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}
