package store

import (
	"context"
	"errors"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-182: a document update changes only the fields it is given, and the
// document and its labels commit together.
func TestUpdateDocumentMergePatch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	str := func(v string) *string { return &v }

	is, err := s.CreateIssue(ctx, ws, IssueInput{Title: "doc target", StateName: "Backlog"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.CreateDocument(ctx, ws,
		models.Document{Title: "T", BodyMD: "body", Type: "decision", Author: "ai", IssueID: &is.ID},
		DocLabels{Set: true, Names: []string{"bug"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Labels) != 1 || doc.Type != "decision" {
		t.Fatalf("create: %+v", doc)
	}

	// Body only: title, type, issue link, author and labels are kept.
	got, err := s.UpdateDocument(ctx, ws, doc.ID, DocumentPatch{BodyMD: str("new body")})
	if err != nil {
		t.Fatal(err)
	}
	if got.BodyMD != "new body" || got.Title != "T" || got.Type != "decision" || got.Author != "ai" ||
		got.IssueID == nil || *got.IssueID != is.ID || len(got.Labels) != 1 {
		t.Fatalf("body-only patch: %+v", got)
	}

	// An invalid label rolls the whole save back: the body is unchanged.
	if _, err := s.UpdateDocument(ctx, ws, doc.ID, DocumentPatch{
		BodyMD: str("should not stick"), Labels: DocLabels{Set: true, IDs: []string{"00000000-0000-0000-0000-000000000000"}},
	}); err == nil {
		t.Fatal("invalid label accepted")
	}
	got, _ = s.GetDocument(ctx, ws, doc.ID)
	if got.BodyMD != "new body" || len(got.Labels) != 1 {
		t.Fatalf("label error left a partial save: %+v", got)
	}

	// Empty issue detaches; an unknown issue is not found and nothing is saved.
	got, err = s.UpdateDocument(ctx, ws, doc.ID, DocumentPatch{IssueID: str("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.IssueID != nil || got.Title != "T" {
		t.Fatalf("detach: %+v", got)
	}
	if _, err := s.UpdateDocument(ctx, ws, doc.ID,
		DocumentPatch{Title: str("nope"), IssueID: str("00000000-0000-0000-0000-000000000000")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
	got, _ = s.GetDocument(ctx, ws, doc.ID)
	if got.Title != "T" {
		t.Fatalf("not-found update changed the title: %+v", got)
	}
	// An issue of another workspace is not found either.
	other := newWorkspace(t, s)
	foreign, _ := s.CreateIssue(ctx, other, IssueInput{Title: "foreign", StateName: "Backlog"})
	if _, err := s.UpdateDocument(ctx, ws, doc.ID, DocumentPatch{IssueID: &foreign.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign issue: %v", err)
	}

	// Labels replaced only when sent; empty set clears.
	got, err = s.UpdateDocument(ctx, ws, doc.ID, DocumentPatch{Labels: DocLabels{Set: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Labels) != 0 {
		t.Fatalf("labels not cleared: %+v", got.Labels)
	}
	if _, err := s.UpdateDocument(ctx, ws, "00000000-0000-0000-0000-000000000000", DocumentPatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown doc: %v", err)
	}
	if _, err := s.SaveDocument(ctx, ws, models.Document{ID: doc.ID, Title: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("SaveDocument with id: %v", err)
	}
}

// Create applies its labels in the same transaction: a bad label saves nothing.
func TestCreateDocumentLabelErrorSavesNothing(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	if _, err := s.CreateDocument(ctx, ws, models.Document{Title: "ghost"},
		DocLabels{Set: true, IDs: []string{"00000000-0000-0000-0000-000000000000"}}); err == nil {
		t.Fatal("invalid label accepted")
	}
	docs, err := s.ListDocuments(ctx, ws, DocFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("document left behind: %+v", docs)
	}
}
