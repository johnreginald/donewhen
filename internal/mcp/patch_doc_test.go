package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
	"github.com/johnreginald/donewhen/internal/store"
)

// PP-182: save_document with an id changes only the fields sent.
func TestSaveDocumentMergePatch(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)
	bg := context.Background()

	out, isErr := e.call(ctx, "save_issue", map[string]any{"title": "doc issue", "state": "In Progress"})
	if isErr {
		t.Fatal(out)
	}
	var is models.Issue
	_ = json.Unmarshal([]byte(out), &is)

	// Create without a type: default is change.
	out, isErr = e.call(ctx, "save_document", map[string]any{"title": "T", "body": "b", "issue": is.Key})
	if isErr {
		t.Fatal(out)
	}
	var doc models.Document
	_ = json.Unmarshal([]byte(out), &doc)
	if doc.Type != "change" || doc.IssueID == nil || *doc.IssueID != is.ID || doc.Author != "ai" {
		t.Fatalf("create: %+v", doc)
	}
	// Give it a non-default type so a reset would show.
	if out, isErr = e.call(ctx, "save_document", map[string]any{"id": doc.ID, "type": "decision"}); isErr {
		t.Fatal(out)
	}

	get := func() models.Document {
		got, err := e.store.GetDocument(bg, w.ID, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	// Body only: title, type and issue link stay.
	if out, isErr = e.call(ctx, "save_document", map[string]any{"id": doc.ID, "body": "new"}); isErr {
		t.Fatal(out)
	}
	got := get()
	if got.BodyMD != "new" || got.Title != "T" || got.Type != "decision" || got.IssueID == nil {
		t.Fatalf("body-only update: %+v", got)
	}
	// Empty issue detaches.
	if out, isErr = e.call(ctx, "save_document", map[string]any{"id": doc.ID, "issue": ""}); isErr {
		t.Fatal(out)
	}
	if got = get(); got.IssueID != nil || got.Title != "T" {
		t.Fatalf("detach: %+v", got)
	}
	// A missing issue is not found; nothing is saved, on create or update.
	out, isErr = e.call(ctx, "save_document", map[string]any{"title": "orphan", "issue": "T999-999"})
	if !isErr || !strings.Contains(strings.ToLower(out), "not found") {
		t.Fatalf("create with missing issue: err=%v %s", isErr, out)
	}
	out, isErr = e.call(ctx, "save_document", map[string]any{"id": doc.ID, "title": "changed", "issue": is.Key + "9999"})
	if !isErr {
		t.Fatalf("update with missing issue accepted: %s", out)
	}
	if got = get(); got.Title != "T" {
		t.Fatalf("failed update changed the title: %+v", got)
	}
	docs, _ := e.store.ListDocuments(bg, w.ID, store.DocFilter{})
	if len(docs) != 1 {
		t.Fatalf("documents = %d, want 1", len(docs))
	}
}
