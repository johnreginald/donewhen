package api

import (
	"errors"
	"net/http"
	"strings"

	"raenil/internal/auth"
	"raenil/internal/models"
	"raenil/internal/store"
)

// handleListWorkspaces returns the workspaces the caller belongs to. This is
// how a client discovers what it may switch to, so it is deliberately NOT
// workspace-scoped — but it still only ever returns the caller's memberships.
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	items, err := s.store.ListMemberships(r.Context(), u.ID)
	if handleStoreErr(w, err) {
		return
	}
	// A pinned token can only ever act on one workspace; say so rather than
	// offering a switcher full of options it will be refused.
	if pin, ok := auth.TokenPinFrom(r.Context()); ok {
		filtered := items[:0]
		for _, m := range items {
			if m.ID == pin {
				filtered = append(filtered, m)
			}
		}
		items = filtered
	}
	writeJSON(w, 200, orEmpty(items))
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string `json:"name"`
		Slug      string `json:"slug"`
		KeyPrefix string `json:"keyPrefix"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if body.KeyPrefix == "" {
		writeErr(w, http.StatusBadRequest, "keyPrefix required")
		return
	}
	if err := store.ValidatePrefix(body.KeyPrefix, s.store.ReservedPrefix()); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, _ := auth.UserFrom(r.Context())
	ws, err := s.store.CreateWorkspace(r.Context(), body.Name, body.Slug, body.KeyPrefix, u.ID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	// adminOnly resolved the active workspace; only that one may be edited here.
	active := ws(r)
	if id := r.PathValue("id"); id != "" && id != active {
		writeErr(w, http.StatusForbidden, "not the active workspace")
		return
	}
	var body struct {
		Name      *string `json:"name"`
		Slug      *string `json:"slug"`
		KeyPrefix *string `json:"keyPrefix"`
		AIName    *string `json:"aiName"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.KeyPrefix != nil {
		if err := store.ValidatePrefix(*body.KeyPrefix, s.store.ReservedPrefix()); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	updated, err := s.store.UpdateWorkspace(r.Context(), active, body.Name, body.Slug, body.KeyPrefix, body.AIName)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, updated)
}

// handleActivateWorkspace remembers the caller's choice so the next session
// lands in the same place.
func (s *Server) handleActivateWorkspace(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	target, err := s.store.ResolveWorkspace(r.Context(), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	role, err := s.store.RoleIn(r.Context(), target.ID, u.ID)
	if handleStoreErr(w, err) {
		return
	}
	if err := s.store.SetLastWorkspace(r.Context(), u.ID, target.ID); err != nil {
		internalErr(w, err)
		return
	}
	writeJSON(w, 200, models.Membership{Workspace: target, Role: role})
}

// ---- members ----

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	members, err := s.store.ListMembers(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(members))
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Email) == "" {
		writeErr(w, http.StatusBadRequest, "email required")
		return
	}
	// v1 grants access to accounts that already exist; there is no invite flow.
	u, _, err := s.store.GetUserByEmail(r.Context(), body.Email)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "no account with that email; create it with `raenil user` first")
		return
	}
	if handleStoreErr(w, err) {
		return
	}
	if err := s.store.AddMember(r.Context(), ws(r), u.ID, body.Role); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"userId": u.ID, "email": u.Email})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	err := s.store.RemoveMember(r.Context(), ws(r), r.PathValue("userId"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not a member")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "removed"})
}
