package api

import (
	"encoding/json"
	"testing"

	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/version"
)

func TestHealthReportsBuild(t *testing.T) {
	e := newEnv(t, config.Config{})
	w := e.do("GET", "/api/health", nil)
	if w.Code != 200 {
		t.Fatalf("health = %d", w.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "ok" {
		t.Errorf("status = %q", got["status"])
	}
	if _, ok := got["builtAt"]; !ok {
		t.Error("builtAt is missing")
	}
	// A test binary has no ldflags: the commit is the default.
	if got["commit"] != "dev" || got["commit"] != version.Commit {
		t.Errorf("commit = %q, want dev", got["commit"])
	}
}
