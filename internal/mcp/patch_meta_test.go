package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/johnreginald/donewhen/internal/models"
)

// PP-181: save_project / save_initiative with an id change only the fields sent.
func TestSaveProjectAndInitiativeMergePatch(t *testing.T) {
	e := newEnv(t)
	uid := e.user()
	w := e.workspace(uid)
	ctx := e.ctxFor(uid, w.ID)
	bg := context.Background()

	var ini models.Initiative
	out, isErr := e.call(ctx, "save_initiative", map[string]any{"name": "ini", "description": "ini desc"})
	if isErr {
		t.Fatal(out)
	}
	_ = json.Unmarshal([]byte(out), &ini)

	var p models.Project
	out, isErr = e.call(ctx, "save_project", map[string]any{
		"name": "epic", "description": "desc", "initiative": ini.ID, "repoUrl": "https://github.com/x/y",
	})
	if isErr {
		t.Fatal(out)
	}
	_ = json.Unmarshal([]byte(out), &p)
	if p.Status != "active" || p.RepoURL == nil {
		t.Fatalf("create: %+v", p)
	}
	if _, err := e.store.ArchiveProject(bg, w.ID, p.ID, true, "human"); err != nil {
		t.Fatal(err)
	}

	get := func() models.Project {
		got, err := e.store.GetProject(bg, w.ID, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// Name only.
	if out, isErr = e.call(ctx, "save_project", map[string]any{"id": p.ID, "name": "X"}); isErr {
		t.Fatal(out)
	}
	got := get()
	if got.Name != "X" || got.DescriptionMD != "desc" || got.Status != "archived" ||
		got.InitiativeID == nil || got.RepoURL == nil {
		t.Fatalf("name-only update: %+v", got)
	}
	// Empty description clears only that.
	if out, isErr = e.call(ctx, "save_project", map[string]any{"id": p.ID, "description": ""}); isErr {
		t.Fatal(out)
	}
	got = get()
	if got.DescriptionMD != "" || got.Name != "X" || got.InitiativeID == nil {
		t.Fatalf("clear description: %+v", got)
	}
	// Empty initiative detaches.
	if out, isErr = e.call(ctx, "save_project", map[string]any{"id": p.ID, "initiative": ""}); isErr {
		t.Fatal(out)
	}
	if got = get(); got.InitiativeID != nil || got.Name != "X" {
		t.Fatalf("detach: %+v", got)
	}
	// Unknown id.
	if out, isErr = e.call(ctx, "save_project", map[string]any{"id": "00000000-0000-0000-0000-000000000000", "name": "x"}); !isErr {
		t.Fatalf("unknown id accepted: %s", out)
	}

	// Initiative: name only keeps the description.
	if out, isErr = e.call(ctx, "save_initiative", map[string]any{"id": ini.ID, "name": "ini2"}); isErr {
		t.Fatal(out)
	}
	gi, err := e.store.GetInitiative(bg, w.ID, ini.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gi.Name != "ini2" || gi.DescriptionMD != "ini desc" {
		t.Fatalf("initiative update: %+v", gi)
	}
}
