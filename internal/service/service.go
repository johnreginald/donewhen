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
	return is, nil
}

// DeleteIssue removes an issue and publishes issue.deleted.
func (s *Service) DeleteIssue(ctx context.Context, id string, actor string) error {
	if err := s.Store.DeleteIssue(ctx, id); err != nil {
		return err
	}
	s.Bus.Publish(events.Event{Type: events.IssueDeleted, Actor: actor, IssueID: id})
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
	}
	return c, nil
}
