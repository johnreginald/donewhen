package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/config"
)

// Tokens minted before the rename to DoneWhen start with "raenil_". Lookup is
// by hash, so they must keep authenticating; new tokens start with "donewhen_".
func TestLegacyTokenPrefixStillAuthenticates(t *testing.T) {
	e := newEnv(t, config.Config{})
	uid, _ := e.user("")

	raw, _ := auth.RandomToken(32)
	legacy := "raenil_" + raw
	if _, err := e.store.CreateAPIToken(context.Background(), uid, "legacy", auth.HashToken(legacy), nil); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/workspaces", nil, bearer(legacy)); w.Code != 200 {
		t.Fatalf("legacy raenil_ token = %d (%s), want 200", w.Code, w.Body.String())
	}

	w := e.do("POST", "/api/tokens", map[string]string{"name": "new"}, browser(e.session(uid)))
	if w.Code != 201 {
		t.Fatalf("create token = %d (%s)", w.Code, w.Body.String())
	}
	var created struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Secret, "donewhen_") {
		t.Fatalf("new token %q does not start with donewhen_", created.Secret)
	}
	if w := e.do("GET", "/api/workspaces", nil, bearer(created.Secret)); w.Code != 200 {
		t.Fatalf("new token = %d, want 200", w.Code)
	}
}
