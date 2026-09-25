package api

import (
	"context"
	"log"
	"time"

	"raenil/internal/auth"
	"raenil/internal/events"
	"raenil/internal/store"
)

// Tick runs what is due each minute: agents' heartbeats.
func (s *Server) Tick(ctx context.Context) {
	now := time.Now()
	beats, err := s.store.ClaimDueHeartbeats(ctx, now)
	if err != nil {
		log.Printf("heartbeats: %v", err)
	}
	for _, b := range beats {
		issues, err := s.store.WaitingForAgent(ctx, b.WorkspaceID, b.AgentID)
		if err != nil {
			log.Printf("heartbeat %s: %v", b.AgentID, err)
			continue
		}
		for _, issueID := range issues {
			j, err := s.store.EnqueueJob(ctx, b.WorkspaceID, store.JobInput{
				Kind: "chat", AgentID: b.AgentID, IssueID: issueID, Input: map[string]any{"reason": "message"},
			})
			if err != nil {
				log.Printf("heartbeat %s: %v", b.AgentID, err)
				continue
			}
			s.bus.Publish(events.Event{Type: "job.updated", WorkspaceID: b.WorkspaceID, Actor: auth.ActorAI, Job: &j, IssueID: issueID})
		}
	}
}
