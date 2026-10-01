// Package service wraps the store with domain-event emission so that every
// issue mutation — whether it comes from the REST API or the MCP server —
// publishes the same events to the bus (SSE + Web Push).
//
// Like the store, every method takes an explicit workspace id: the event a
// mutation publishes carries it, which is what keeps one workspace's live
// updates out of another's board.
package service

import (
	"context"

	"github.com/johnreginald/donewhen/internal/events"
	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

type Service struct {
	Store *store.Store
	Bus   *events.Bus
}

func New(s *store.Store, bus *events.Bus) *Service {
	return &Service{Store: s, Bus: bus}
}

func (s *Service) statePtr(ctx context.Context, wsID, id string) *models.WorkflowState {
	if id == "" {
		return nil
	}
	st, err := s.Store.GetState(ctx, wsID, id)
	if err != nil {
		return nil
	}
	return &st
}

var prioLabels = map[int]string{0: "No priority", 1: "Urgent", 2: "High", 3: "Medium", 4: "Low"}

func (s *Service) stateName(ctx context.Context, wsID, id string) string {
	if st := s.statePtr(ctx, wsID, id); st != nil {
		return st.Name
	}
	return ""
}

func (s *Service) projName(ctx context.Context, wsID string, id *string) string {
	if id == nil || *id == "" {
		return "None"
	}
	p, err := s.Store.GetProject(ctx, wsID, *id)
	if err != nil {
		return ""
	}
	return p.Name
}

// logActivity writes one timeline entry, best-effort (never fails the caller).
func (s *Service) logActivity(ctx context.Context, wsID string, a models.Activity) {
	_ = s.Store.RecordActivity(ctx, wsID, a)
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func excerpt(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// CreateIssue persists a new issue and publishes issue.created.
func (s *Service) CreateIssue(ctx context.Context, wsID string, in store.IssueInput, actor string) (models.Issue, error) {
	var gate store.GateOutcome
	in.ForceGate = in.ForceGate && actor != "ai" // AI callers never force
	in.GateOut = &gate
	is, err := s.Store.CreateIssue(ctx, wsID, in)
	if err != nil {
		return is, err
	}
	defer s.logOverride(ctx, wsID, is, gate, actor)
	s.Bus.Publish(events.Event{
		Type:        events.IssueCreated,
		WorkspaceID: wsID,
		Actor:       actor,
		Issue:       &is,
		To:          s.statePtr(ctx, wsID, is.StateID),
	})
	s.logActivity(ctx, wsID, models.Activity{
		IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
		Kind: "created", ToVal: s.stateName(ctx, wsID, is.StateID),
	})
	return is, nil
}

// UpdateIssue applies a patch and publishes issue.state_changed when the state
// moved, otherwise issue.updated.
func (s *Service) UpdateIssue(ctx context.Context, wsID, id string, p store.IssuePatch, actor string) (models.Issue, error) {
	var before models.Issue // read inside the update transaction, under the row lock
	var gate store.GateOutcome
	p.ForceGate = p.ForceGate && actor != "ai" // AI callers never force
	p.GateOut = &gate
	p.BeforeOut = &before
	is, err := s.Store.UpdateIssue(ctx, wsID, id, p)
	if err != nil {
		return is, err
	}
	defer s.logOverride(ctx, wsID, is, gate, actor)
	if before.StateID != is.StateID {
		s.Bus.Publish(events.Event{
			Type:        events.IssueStateChanged,
			WorkspaceID: wsID,
			Actor:       actor,
			Issue:       &is,
			From:        s.statePtr(ctx, wsID, before.StateID),
			To:          s.statePtr(ctx, wsID, is.StateID),
		})
	} else {
		s.Bus.Publish(events.Event{
			Type:        events.IssueUpdated,
			WorkspaceID: wsID,
			Actor:       actor,
			Issue:       &is,
		})
	}
	// Timeline: one entry per meaningful field change.
	base := models.Activity{IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor}
	if before.StateID != is.StateID {
		e := base
		e.Kind, e.Field = "state_changed", "status"
		e.FromVal, e.ToVal = s.stateName(ctx, wsID, before.StateID), s.stateName(ctx, wsID, is.StateID)
		s.logActivity(ctx, wsID, e)
	}
	if before.Priority != is.Priority {
		e := base
		e.Kind, e.Field = "priority_changed", "priority"
		e.FromVal, e.ToVal = prioLabels[before.Priority], prioLabels[is.Priority]
		s.logActivity(ctx, wsID, e)
	}
	if ptrStr(before.ProjectID) != ptrStr(is.ProjectID) {
		e := base
		e.Kind, e.Field = "epic_changed", "epic"
		e.FromVal, e.ToVal = s.projName(ctx, wsID, before.ProjectID), s.projName(ctx, wsID, is.ProjectID)
		s.logActivity(ctx, wsID, e)
	}
	if before.Title != is.Title {
		e := base
		e.Kind, e.Field = "title_changed", "title"
		e.FromVal, e.ToVal = before.Title, is.Title
		s.logActivity(ctx, wsID, e)
	}
	return is, nil
}

// logOverride records a forced move past the done-when gate.
func (s *Service) logOverride(ctx context.Context, wsID string, is models.Issue, g store.GateOutcome, actor string) {
	if !g.Overridden {
		return
	}
	s.logActivity(ctx, wsID, models.Activity{
		IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
		Kind: "gate_overridden", Field: "status",
		ToVal:  s.stateName(ctx, wsID, is.StateID),
		Detail: g.Err.Error(),
	})
}

// DeleteIssue removes an issue and publishes issue.deleted.
func (s *Service) DeleteIssue(ctx context.Context, wsID, id string, actor string) error {
	before, _ := s.Store.GetIssue(ctx, wsID, id) // snapshot for the record
	if err := s.Store.DeleteIssue(ctx, wsID, id); err != nil {
		return err
	}
	s.Bus.Publish(events.Event{
		Type: events.IssueDeleted, WorkspaceID: wsID, Actor: actor, IssueID: id,
	})
	s.logActivity(ctx, wsID, models.Activity{
		IssueKey: before.Key, IssueTitle: before.Title, Actor: actor, Kind: "deleted",
	})
	return nil
}

// AddComment stores a comment and publishes comment.added.
func (s *Service) AddComment(ctx context.Context, wsID, issueID, body, actor string) (models.Comment, error) {
	c, err := s.Store.CreateComment(ctx, wsID, issueID, body, actor)
	if err != nil {
		return c, err
	}
	is, err := s.Store.GetIssue(ctx, wsID, issueID)
	if err == nil {
		s.Bus.Publish(events.Event{
			Type:        events.CommentAdded,
			WorkspaceID: wsID,
			Actor:       actor,
			Issue:       &is,
			Comment:     &c,
		})
		s.logActivity(ctx, wsID, models.Activity{
			IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
			Kind: "commented", Detail: excerpt(body, 100),
		})
	}
	return c, nil
}
