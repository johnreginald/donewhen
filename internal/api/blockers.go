package api

import (
	"context"
	"log"
	"net/http"
	"strings"

	"raenil/internal/auth"
	"raenil/internal/events"
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
// Done, until the epic is stopped.
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
		// A run with nothing to check it against is refused by the host;
		// queueing it would only fail.
		if crit, err := s.store.ListCriteria(ctx, wsID, is.ID); err != nil {
			return res, err
		} else if len(crit) == 0 {
			res.NoCheck = append(res.NoCheck, is.Key)
			continue
		}
		ref, err := s.store.IssueAgentRef(ctx, wsID, is)
		if err != nil {
			return res, err
		}
		if ref == "" {
			res.NoAgent = append(res.NoAgent, is.Key)
			continue
		}
		a, err := s.store.GetAgent(ctx, wsID, ref)
		if err != nil || a.Status == "paused" {
			res.NoAgent = append(res.NoAgent, is.Key)
			continue
		}
		j, err := s.store.EnqueueJob(ctx, wsID, store.JobInput{Kind: "run_ticket", AgentID: a.ID, IssueID: is.ID})
		if err != nil {
			// Over budget, most likely: the rest may still go.
			log.Printf("epic %s: %s not queued: %v", epicID, is.Key, err)
			continue
		}
		res.Queued = append(res.Queued, is.Key)
		s.bus.Publish(events.Event{Type: "job.updated", WorkspaceID: wsID, Actor: actor, Job: &j, IssueID: is.ID})
	}
	return res, nil
}

// WatchEpics starts the next tickets of a running epic when one it waits on
// is Done. It listens to every workspace's events, like the notifier.
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
				(e.To.Category != "completed" && e.To.Category != "canceled") {
				continue
			}
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
					log.Printf("epic %s: %s Done, started %s", epic, e.Issue.Key, strings.Join(res.Queued, ", "))
				}
			}
		}
	}
}
