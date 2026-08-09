package api

import (
	"net/http"

	"raenil/internal/auth"
	"raenil/internal/models"
)

// ---- commits ----

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.ListCommits(r.Context(), ws(r), r.PathValue("id"))
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
	id := r.PathValue("id")
	c, err := s.store.AddCommit(r.Context(), ws(r), id, body.Sha, body.Message, strPtr(body.URL))
	if handleStoreErr(w, err) {
		return
	}
	if is, e := s.store.GetIssue(r.Context(), ws(r), id); e == nil {
		_ = s.store.RecordActivity(r.Context(), ws(r), models.Activity{
			IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title,
			Actor: auth.ActorFrom(r.Context()), Kind: "committed",
			Detail: shortSHA(body.Sha) + " " + body.Message,
		})
	}
	writeJSON(w, http.StatusCreated, c)
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// ---- dev links (branch / PR) ----

func (s *Server) handleSetDev(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GitBranch string `json:"gitBranch"`
		PrURL     string `json:"prUrl"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	is, err := s.store.SetIssueDev(r.Context(), ws(r), r.PathValue("id"), strPtr(body.GitBranch), strPtr(body.PrURL))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, is)
}

// ---- done-when criteria ----

func (s *Server) handleListCriteria(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.ListCriteria(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(cs))
}

func (s *Server) handleAddCriterion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &body); err != nil || body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body required")
		return
	}
	c, err := s.store.AddCriterion(r.Context(), ws(r), r.PathValue("id"), body.Body)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCriterion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body *string `json:"body"`
		Done *bool   `json:"done"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	c, err := s.store.UpdateCriterion(r.Context(), ws(r), r.PathValue("id"), body.Body, body.Done)
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
// actually does". Without it, a tool holding a SHA has no way to ask Raenil
// what that commit was supposed to accomplish.
//
// 404 when no issue claims the SHA — an ordinary outcome, not an error.
func (s *Server) handleIssueByCommit(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha")
	if sha == "" {
		writeErr(w, http.StatusBadRequest, "sha required")
		return
	}
	owner, err := s.store.IssueByCommit(r.Context(), []string{ws(r)}, sha)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, owner)
}
