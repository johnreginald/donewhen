package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

// ---- "blocked by" links ----

func (s *Server) handleGetBlockers(w http.ResponseWriter, r *http.Request) {
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	s.writeBlockers(w, r, is.ID)
}

func (s *Server) writeBlockers(w http.ResponseWriter, r *http.Request, issueID string) {
	by, err := s.store.ListBlockers(r.Context(), ws(r), issueID)
	if handleStoreErr(w, err) {
		return
	}
	blocking, err := s.store.ListBlocking(r.Context(), ws(r), issueID)
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blockedBy": by, "blocking": blocking})
}

// handleSetBlockers replaces the tickets that block one ticket.
func (s *Server) handleSetBlockers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BlockedBy []string `json:"blockedBy"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	is, err := s.resolveIssue(r, r.PathValue("id"))
	if handleStoreErr(w, err) {
		return
	}
	if _, err := s.store.SetBlockers(r.Context(), ws(r), is.ID, body.BlockedBy); handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "issue.blockers", IssueID: is.ID})
	s.writeBlockers(w, r, is.ID)
}

// handleListBlockLinks lists every "blocked by" edge in the workspace, for
// the board.
func (s *Server) handleListBlockLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.store.ListBlockLinks(r.Context(), ws(r))
	if handleStoreErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// ---- running an epic ----

// EpicRun says what starting an epic did.
type EpicRun struct {
	Queued  []string `json:"queued"`   // started now
	Blocked []string `json:"blocked"`  // Ready, waiting on a blocker
	NoAgent []string `json:"noAgent"`  // Ready, but no agent and no default for its priority
	NoCheck []string `json:"noChecks"` // Ready, but no done-when to check the work against
}

// handleRunEpic sets an epic running: every Ready ticket in it that nothing
// blocks starts now, and each of the rest starts as soon as its blockers are
// In Review or Done, until the epic is stopped.
func (s *Server) handleRunEpic(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if handleStoreErr(w, s.store.SetEpicAutorun(r.Context(), ws(r), id, true)) {
		return
	}
	res, err := s.runEpic(r.Context(), ws(r), id, auth.ActorHuman)
	if handleStoreErr(w, err) {
		return
	}
	s.publish(r, events.Event{Type: "project.running"})
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleStopEpic(w http.ResponseWriter, r *http.Request) {
	if handleStoreErr(w, s.store.SetEpicAutorun(r.Context(), ws(r), r.PathValue("id"), false)) {
		return
	}
	s.publish(r, events.Event{Type: "project.running"})
	w.WriteHeader(http.StatusNoContent)
}

// runEpic queues a run for each of the epic's Ready tickets that nothing
// blocks, each on its own agent or its priority's default.
func (s *Server) runEpic(ctx context.Context, wsID, epicID, actor string) (EpicRun, error) {
	res := EpicRun{Queued: []string{}, Blocked: []string{}, NoAgent: []string{}, NoCheck: []string{}}
	ready, blocked, err := s.store.ReadyToRun(ctx, wsID, epicID)
	if err != nil {
		return res, err
	}
	res.Blocked = append(res.Blocked, blocked...)
	for _, is := range ready {
		switch why, err := s.queueRun(ctx, wsID, is, actor); {
		case err != nil:
			return res, err
		case why == "":
			res.Queued = append(res.Queued, is.Key)
		case why == "no checks":
			res.NoCheck = append(res.NoCheck, is.Key)
		case why == "no agent":
			res.NoAgent = append(res.NoAgent, is.Key)
		default:
			log.Printf("epic %s: %s not queued: %s", epicID, is.Key, why)
		}
	}
	return res, nil
}

// queueRun queues a run of one ticket on its agent (or its priority's
// default). It returns why it did not, when it did not: "no checks" — a run
// with nothing to check it against would only be refused — "no agent", or
// the reason the queue refused it (a budget, most likely).
func (s *Server) queueRun(ctx context.Context, wsID string, is models.Issue, actor string) (string, error) {
	crit, err := s.store.ListCriteria(ctx, wsID, is.ID)
	if err != nil {
		return "", err
	}
	if len(crit) == 0 {
		return "no checks", nil
	}
	ref, err := s.store.IssueAgentRef(ctx, wsID, is)
	if err != nil {
		return "", err
	}
	if ref == "" {
		return "no agent", nil
	}
	a, err := s.store.GetAgent(ctx, wsID, ref)
	if err != nil || a.Status == "paused" {
		return "no agent", nil
	}
	j, err := s.store.EnqueueJob(ctx, wsID, store.JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is.ID})
	if err != nil {
		return err.Error(), nil
	}
	s.bus.Publish(events.Event{Type: "job.updated", WorkspaceID: wsID, Actor: actor, Job: &j, IssueID: is.ID})
	return "", nil
}

// runWaiting starts the tickets that were waiting on this one and asked to
// start again on their own, now that nothing is in their way.
func (s *Server) runWaiting(ctx context.Context, wsID, blockerID string) {
	waiting, err := s.store.UnblockedWaiting(ctx, wsID, blockerID)
	if err != nil {
		log.Printf("waiting tickets: %v", err)
		return
	}
	for _, is := range waiting {
		if why, err := s.queueRun(ctx, wsID, is, auth.ActorAI); err != nil || why != "" {
			log.Printf("%s was waiting and is free, but not started: %v %s", is.Key, err, why)
			continue
		}
		_ = s.store.ClearRunWhenUnblocked(ctx, wsID, is.ID)
		log.Printf("%s: its blocker cleared, started again", is.Key)
	}
}

// WatchEpics starts the next tickets of a running epic when one it waits on
// reaches In Review or Done. It listens to every workspace's events, like the notifier.
func (s *Server) WatchEpics(ctx context.Context) {
	ch, unsub := s.bus.Subscribe("")
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if e.Type != events.IssueStateChanged || e.Issue == nil || e.To == nil ||
				(e.To.Category != "completed" && e.To.Category != "canceled" && e.To.Name != "In Review") {
				continue
			}
			s.runWaiting(ctx, e.WorkspaceID, e.Issue.ID)
			epics, err := s.store.AutorunDependents(ctx, e.WorkspaceID, e.Issue.ID)
			if err != nil {
				log.Printf("epics: %v", err)
				continue
			}
			for _, epic := range epics {
				res, err := s.runEpic(ctx, e.WorkspaceID, epic, auth.ActorAI)
				if err != nil {
					log.Printf("epic %s: %v", epic, err)
				} else if len(res.Queued) > 0 {
					log.Printf("epic %s: %s cleared, started %s", epic, e.Issue.Key, strings.Join(res.Queued, ", "))
				}
			}
		}
	}
}
