package api

import (
	"encoding/json"
	"net/http"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/store"
)

// ---- commits ----

// issueID resolves the {id} path segment, which callers may give as a human key
// (PP-42) or a UUID. The store only accepts UUIDs, so a handler that passes the
// raw path value rejects every key with an opaque "invalid input syntax for type
// uuid" — which is what /api/issues/{id} itself does not do.
func (s *Server) issueID(r *http.Request) (string, error) {
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if err != nil {
		return "", err
	}
	return is.ID, nil
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	cs, err := s.store.ListCommits(r.Context(), ws(r), id)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(cs))
}

func (s *Server) handleAddCommit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Sha     string `json:"sha"`
		Message string `json:"message"`
		URL     string `json:"url"`
	}
	if err := readJSON(r, &body); err != nil || body.Sha == "" {
		writeErr(w, http.StatusBadRequest, "sha required")
		return
	}
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	c, err := s.store.AddCommitAs(r.Context(), ws(r), id, body.Sha, body.Message, strPtr(body.URL), auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// ---- dev links (branch / PR) ----

func (s *Server) handleSetDev(w http.ResponseWriter, r *http.Request) {
	// Only the fields present change; null or "" clears that field.
	var body struct {
		GitBranch json.RawMessage `json:"gitBranch"`
		PrURL     json.RawMessage `json:"prUrl"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	branch, ok1 := patchString(body.GitBranch)
	pr, ok2 := patchString(body.PrURL)
	if !ok1 || !ok2 {
		writeErr(w, http.StatusBadRequest, "gitBranch and prUrl must be strings or null")
		return
	}
	if branch == nil && pr == nil {
		writeErr(w, http.StatusBadRequest, "nothing to set: send gitBranch and/or prUrl")
		return
	}
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	is, err := s.store.SetIssueDev(r.Context(), ws(r), id, branch, pr)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, is)
}

// ---- done-when criteria ----

// validateCriterionSpec is the shared store rule; see store.ValidateCriterionSpec.
func validateCriterionSpec(kind string, spec json.RawMessage) error {
	return store.ValidateCriterionSpec(kind, spec)
}

func (s *Server) handleListCriteria(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	cs, err := s.store.ListCriteria(r.Context(), ws(r), id)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(cs))
}

func (s *Server) handleAddCriterion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body      string          `json:"body"`
		Kind      string          `json:"kind"`
		CheckSpec json.RawMessage `json:"checkSpec"`
	}
	if err := readJSON(r, &body); err != nil || body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body required")
		return
	}
	if err := validateCriterionSpec(body.Kind, body.CheckSpec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	c, err := s.store.AddCriterion(r.Context(), ws(r), id, body.Body, body.Kind, body.CheckSpec)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCriterion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body        *string         `json:"body"`
		Done        *bool           `json:"done"`
		Kind        *string         `json:"kind"`
		CheckSpec   json.RawMessage `json:"checkSpec"`
		EvidenceRef *string         `json:"evidenceRef"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Kind != nil {
		if err := validateCriterionSpec(*body.Kind, body.CheckSpec); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	c, err := s.store.UpdateCriterionAs(r.Context(), ws(r), r.PathValue("id"), body.Body, body.Done, body.Kind, body.CheckSpec, body.EvidenceRef, auth.ActorFrom(r.Context()))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) handleDeleteCriterion(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.DeleteCriterion(r.Context(), ws(r), r.PathValue("id"))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleIssueByCommit handles GET /api/commits/{sha} → the issue that
// recorded this commit, with its done-when criteria.
//
// The reverse of POST /api/issues/{id}/commits, and the seam a code
// intelligence tool needs: both systems already record a commit SHA, so it
// is the one key that joins "why this was built" to "what the code
// actually does". Without it, a tool holding a SHA has no way to ask DoneWhen
// what that commit was supposed to accomplish.
//
// 404 when no issue claims the SHA — an ordinary outcome, not an error.
func (s *Server) handleIssueByCommit(w http.ResponseWriter, r *http.Request) {
	owner, err := s.store.IssueByCommit(r.Context(), []string{ws(r)}, r.PathValue("sha"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, owner)
}
