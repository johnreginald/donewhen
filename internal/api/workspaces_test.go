package api

import (
	"context"
	"testing"

	"raenil/internal/config"
)

// PP-189 on the REST side: PATCH /api/workspaces/{id} rejects blank fields and
// maps unique-key collisions to 409 instead of 500.
func TestUpdateWorkspaceValidation(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	a := e.workspace(uid)
	other := e.workspace(uid)
	cookies := e.session(uid)

	patch := func(body map[string]any) (int, string) {
		w := e.do("PATCH", "/api/workspaces/"+a.ID, body, browser(cookies), header("X-Workspace", a.ID))
		return w.Code, errBody(w)
	}

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"empty name", map[string]any{"name": ""}, 400},
		{"blank name", map[string]any{"name": "  "}, 400},
		{"empty slug", map[string]any{"slug": ""}, 400},
		{"slug that slugifies to nothing", map[string]any{"slug": "---"}, 400},
		{"empty prefix", map[string]any{"keyPrefix": ""}, 400},
		{"reserved prefix", map[string]any{"keyPrefix": "K"}, 400},
		{"duplicate slug", map[string]any{"slug": other.Slug}, 409},
		{"duplicate prefix", map[string]any{"keyPrefix": other.KeyPrefix}, 409},
		{"valid rename", map[string]any{"name": "Fine " + uniq()}, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := e.store.GetWorkspace(context.Background(), a.ID)
			code, msg := patch(tc.body)
			if code != tc.want {
				t.Fatalf("PATCH %v = %d (%s), want %d", tc.body, code, msg, tc.want)
			}
			after, _ := e.store.GetWorkspace(context.Background(), a.ID)
			if tc.want != 200 && (after.Name != before.Name || after.Slug != before.Slug || after.KeyPrefix != before.KeyPrefix) {
				t.Fatalf("rejected PATCH changed the workspace: %+v -> %+v", before, after)
			}
		})
	}
}

func TestCreateWorkspaceDuplicateIs409(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")
	existing := e.workspace(uid)
	cookies := e.session(uid)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"duplicate slug", map[string]any{"name": "Dup " + uniq(), "slug": existing.Slug, "keyPrefix": "Q" + uniq()}},
		{"duplicate prefix", map[string]any{"name": "Dup " + uniq(), "keyPrefix": existing.KeyPrefix}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := e.do("POST", "/api/workspaces", tc.body, browser(cookies))
			if w.Code != 409 {
				t.Fatalf("= %d (%s), want 409", w.Code, w.Body.String())
			}
		})
	}
}
