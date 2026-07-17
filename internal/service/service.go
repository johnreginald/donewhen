// Package service wraps the store with domain-event emission so that every
// issue mutation — whether it comes from the REST API or the MCP server —
// publishes the same events to the bus (SSE + Web Push).
package service

import (
	"context"

	"raenil/internal/events"
	"raenil/internal/models"
	"raenil/internal/store"
)

type Service struct {
	Store *store.Store
	Bus   *events.Bus
}

func New(s *store.Store, bus *events.Bus) *Service {
	return &Service{Store: s, Bus: bus}
}

func (s *Service) statePtr(ctx context.Context, id string) *models.WorkflowState {
	if id == "" {
		return nil
	}
	st, err := s.Store.GetState(ctx, id)
	if err != nil {
		return nil
	}
	return &st
}

var prioLabels = map[int]string{0: "No priority", 1: "Urgent", 2: "High", 3: "Medium", 4: "Low"}

func (s *Service) stateName(ctx context.Context, id string) string {
	if st := s.statePtr(ctx, id); st != nil {
		return st.Name
	}
	return ""
}

func (s *Service) projName(ctx context.Context, id *string) string {
	if id == nil || *id == "" {
		return "None"
	}
	p, err := s.Store.GetProject(ctx, *id)
	if err != nil {
		return ""
	}
	return p.Name
}

// logActivity writes one timeline entry, best-effort (never fails the caller).
func (s *Service) logActivity(ctx context.Context, a models.Activity) {
	_ = s.Store.RecordActivity(ctx, a)
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
func (s *Service) CreateIssue(ctx context.Context, in store.IssueInput, actor string) (models.Issue, error) {
	is, err := s.Store.CreateIssue(ctx, in)
	if err != nil {
		return is, err
	}
	s.Bus.Publish(events.Event{
		Type:  events.IssueCreated,
		Actor: actor,
		Issue: &is,
		To:    s.statePtr(ctx, is.StateID),
	})
	s.logActivity(ctx, models.Activity{
		IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
		Kind: "created", ToVal: s.stateName(ctx, is.StateID),
	})
	return is, nil
}

// UpdateIssue applies a patch and publishes issue.state_changed when the state
// moved, otherwise issue.updated.
func (s *Service) UpdateIssue(ctx context.Context, id string, p store.IssuePatch, actor string) (models.Issue, error) {
	before, err := s.Store.GetIssue(ctx, id)
	if err != nil {
		return models.Issue{}, err
	}
	is, err := s.Store.UpdateIssue(ctx, id, p)
	if err != nil {
		return is, err
	}
	if before.StateID != is.StateID {
		s.Bus.Publish(events.Event{
			Type:  events.IssueStateChanged,
			Actor: actor,
			Issue: &is,
			From:  s.statePtr(ctx, before.StateID),
			To:    s.statePtr(ctx, is.StateID),
		})
	} else {
		s.Bus.Publish(events.Event{
			Type:  events.IssueUpdated,
			Actor: actor,
			Issue: &is,
		})
	}
	// Timeline: one entry per meaningful field change.
	base := models.Activity{IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor}
	if before.StateID != is.StateID {
		e := base
		e.Kind, e.Field = "state_changed", "status"
		e.FromVal, e.ToVal = s.stateName(ctx, before.StateID), s.stateName(ctx, is.StateID)
		s.logActivity(ctx, e)
	}
	if before.Priority != is.Priority {
		e := base
		e.Kind, e.Field = "priority_changed", "priority"
		e.FromVal, e.ToVal = prioLabels[before.Priority], prioLabels[is.Priority]
		s.logActivity(ctx, e)
	}
	if ptrStr(before.ProjectID) != ptrStr(is.ProjectID) {
		e := base
		e.Kind, e.Field = "epic_changed", "epic"
		e.FromVal, e.ToVal = s.projName(ctx, before.ProjectID), s.projName(ctx, is.ProjectID)
		s.logActivity(ctx, e)
	}
	if before.Title != is.Title {
		e := base
		e.Kind, e.Field = "title_changed", "title"
		e.FromVal, e.ToVal = before.Title, is.Title
		s.logActivity(ctx, e)
	}
	return is, nil
}

// DeleteIssue removes an issue and publishes issue.deleted.
func (s *Service) DeleteIssue(ctx context.Context, id string, actor string) error {
	before, _ := s.Store.GetIssue(ctx, id) // snapshot for the record
	if err := s.Store.DeleteIssue(ctx, id); err != nil {
		return err
	}
	s.Bus.Publish(events.Event{Type: events.IssueDeleted, Actor: actor, IssueID: id})
	s.logActivity(ctx, models.Activity{
		IssueKey: before.Key, IssueTitle: before.Title, Actor: actor, Kind: "deleted",
	})
	return nil
}

// AddComment stores a comment and publishes comment.added.
func (s *Service) AddComment(ctx context.Context, issueID, body, actor string) (models.Comment, error) {
	c, err := s.Store.CreateComment(ctx, issueID, body, actor)
	if err != nil {
		return c, err
	}
	is, err := s.Store.GetIssue(ctx, issueID)
	if err == nil {
		s.Bus.Publish(events.Event{
			Type:    events.CommentAdded,
			Actor:   actor,
			Issue:   &is,
			Comment: &c,
		})
		s.logActivity(ctx, models.Activity{
			IssueID: &is.ID, IssueKey: is.Key, IssueTitle: is.Title, Actor: actor,
			Kind: "commented", Detail: excerpt(body, 100),
		})
	}
	return c, nil
}
