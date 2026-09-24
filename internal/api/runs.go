package api

import (
	"fmt"
	"net/http"
	"strconv"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	f := store.RunFilter{}
	if ref := r.URL.Query().Get("issue"); ref != "" {
		is, err := s.resolveIssue(r, ref)
		if handleStoreErr(w, err) {
			return
		}
		f.IssueID = is.ID
	}
	f.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.store.ListRuns(r.Context(), ws(r), f)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(runs))
}

func (s *Server) handleListIssueRuns(w http.ResponseWriter, r *http.Request) {
	id, err := s.issueID(r)
	if handleStoreErr(w, err) {
		return
	}
	runs, err := s.store.ListRuns(r.Context(), ws(r), store.RunFilter{IssueID: id})
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, orEmpty(runs))
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.store.GetRun(r.Context(), ws(r), r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, 200, run)
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Issue   string `json:"issue"` // id or key
		Agent   string `json:"agent"` // optional agent id
		Runner  string `json:"runner"`
		Model   string `json:"model"`
		Attempt int    `json:"attempt"`
		Host    string `json:"host"`
	}
	if err := readJSON(r, &body); err != nil || body.Issue == "" || body.Runner == "" {
		writeErr(w, http.StatusBadRequest, "issue and runner required")
		return
	}
	is, err := s.resolveIssue(r, body.Issue)
	if handleStoreErr(w, err) {
		return
	}
	run, err := s.store.StartRun(r.Context(), ws(r), store.RunStart{
		IssueID: is.ID, AgentID: body.Agent, Runner: body.Runner, Model: body.Model, Attempt: body.Attempt, Host: body.Host,
	})
	if handleStoreErr(w, err) {
		return
	}
	s.publishRun(r, "run.started", run)
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleFinishRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status      string           `json:"status"`
		Verdict     string           `json:"verdict"`
		SessionID   string           `json:"sessionId"`
		ExitCode    *int             `json:"exitCode"`
		AgentError  string           `json:"agentError"`
		Tokens      models.RunTokens `json:"tokens"`
		CostUSD     float64          `json:"costUsd"`
		NotionalUSD float64          `json:"notionalCostUsd"`
		Billing     string           `json:"billing"`
		DeniedTools []string         `json:"deniedTools"`
		LogTail     string           `json:"logTail"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	run, err := s.store.FinishRun(r.Context(), ws(r), r.PathValue("id"), store.RunFinish{
		Status: body.Status, Verdict: body.Verdict, SessionID: body.SessionID, ExitCode: body.ExitCode,
		AgentError: body.AgentError, Tokens: body.Tokens, CostUSD: body.CostUSD, NotionalUSD: body.NotionalUSD,
		Billing: body.Billing, DeniedTools: body.DeniedTools, LogTail: body.LogTail,
	})
	if handleStoreErr(w, err) {
		return
	}
	if run.IssueID != nil {
		detail := fmt.Sprintf("%s · %s", run.Runner, run.Status)
		if run.Verdict != "" {
			detail += " · " + run.Verdict
		}
		_ = s.store.RecordActivity(r.Context(), ws(r), models.Activity{
			IssueID: run.IssueID, IssueKey: run.IssueKey, IssueTitle: run.IssueTitle,
			Actor: auth.ActorAI, Kind: "ran", Detail: detail,
		})
	}
	run.LogTail = "" // the event is a notification, not the transcript
	s.publishRun(r, "run.finished", run)
	writeJSON(w, 200, run)
}

func (s *Server) publishRun(r *http.Request, kind string, run models.Run) {
	e := events.Event{Type: kind, WorkspaceID: ws(r), Actor: auth.ActorAI, Run: &run}
	if run.IssueID != nil {
		e.IssueID = *run.IssueID
	}
	s.bus.Publish(e)
}
