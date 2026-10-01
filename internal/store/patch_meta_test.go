package store

import (
	"context"
	"errors"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-181: an update changes only the fields it is given.
func TestUpdateProjectMergePatch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	str := func(v string) *string { return &v }

	ini, err := s.SaveInitiative(ctx, ws, models.Initiative{Name: "ini", DescriptionMD: "ini desc"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.SaveProject(ctx, ws, models.Project{
		Name: "epic", DescriptionMD: "desc", InitiativeID: &ini.ID,
		RepoURL: str("https://github.com/x/y"), Position: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "active" {
		t.Fatalf("create default status = %q", p.Status)
	}
	if _, err := s.ArchiveProject(ctx, ws, p.ID, true, "human"); err != nil {
		t.Fatal(err)
	}

	// Only the name: everything else, archived status included, is kept.
	got, err := s.UpdateProject(ctx, ws, p.ID, ProjectPatch{Name: str("renamed")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" || got.DescriptionMD != "desc" || got.Status != "archived" || got.Position != 7 ||
		got.InitiativeID == nil || *got.InitiativeID != ini.ID || got.RepoURL == nil || *got.RepoURL != "https://github.com/x/y" {
		t.Fatalf("after name-only patch: %+v", got)
	}

	// Empty description clears just the description.
	got, err = s.UpdateProject(ctx, ws, p.ID, ProjectPatch{DescriptionMD: str("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.DescriptionMD != "" || got.Name != "renamed" || got.RepoURL == nil || got.InitiativeID == nil {
		t.Fatalf("after clearing description: %+v", got)
	}

	// Empty repoUrl clears the repo; empty initiative detaches.
	got, err = s.UpdateProject(ctx, ws, p.ID, ProjectPatch{RepoURL: str(""), InitiativeID: str("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != nil || got.InitiativeID != nil || got.Name != "renamed" || got.Status != "archived" {
		t.Fatalf("after clear/detach: %+v", got)
	}

	// Only an id: a no-op that returns the current row.
	before, _ := s.GetProject(ctx, ws, p.ID)
	got, err = s.UpdateProject(ctx, ws, p.ID, ProjectPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != before.Name || !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("empty patch changed the row: %+v vs %+v", got, before)
	}

	// Unknown id, foreign initiative.
	if _, err := s.UpdateProject(ctx, ws, "00000000-0000-0000-0000-000000000000", ProjectPatch{Name: str("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	other := newWorkspace(t, s)
	foreign, _ := s.SaveInitiative(ctx, other, models.Initiative{Name: "foreign"})
	if _, err := s.UpdateProject(ctx, ws, p.ID, ProjectPatch{InitiativeID: &foreign.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign initiative: %v", err)
	}
	// Create is not an update.
	if _, err := s.SaveProject(ctx, ws, models.Project{ID: p.ID, Name: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("SaveProject with id: %v", err)
	}
}

func TestUpdateInitiativeMergePatch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ws := newWorkspace(t, s)
	str := func(v string) *string { return &v }

	ini, err := s.SaveInitiative(ctx, ws, models.Initiative{
		Name: "ini", DescriptionMD: "keep me", RepoURL: str("https://github.com/x/y"), Position: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ini.Status != "active" {
		t.Fatalf("create default status = %q", ini.Status)
	}
	got, err := s.UpdateInitiative(ctx, ws, ini.ID, InitiativePatch{Name: str("renamed")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" || got.DescriptionMD != "keep me" || got.Position != 3 || got.Status != "active" ||
		got.RepoURL == nil || *got.RepoURL != "https://github.com/x/y" {
		t.Fatalf("after name-only patch: %+v", got)
	}
	got, err = s.UpdateInitiative(ctx, ws, ini.ID, InitiativePatch{DescriptionMD: str(""), RepoURL: str("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.DescriptionMD != "" || got.RepoURL != nil || got.Name != "renamed" {
		t.Fatalf("after clearing: %+v", got)
	}
	if _, err := s.UpdateInitiative(ctx, ws, "00000000-0000-0000-0000-000000000000", InitiativePatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := s.UpdateInitiative(ctx, ws, ini.ID, InitiativePatch{RepoURL: str("ftp://nope")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad url: %v", err)
	}
}
