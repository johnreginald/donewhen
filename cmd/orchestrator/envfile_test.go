package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "orchestrator.env")
	os.WriteFile(p, []byte(`# settings
RAENIL_URL=https://raenil.example
export ORCH_T_QUOTED="a b"
ORCH_T_SINGLE='c=d'
ORCH_T_KEEP=from-file

not a line
`), 0o600)
	t.Setenv("ORCH_T_KEEP", "from-shell")
	t.Setenv("RAENIL_URL", "") // set, even if empty: the shell wins
	os.Unsetenv("ORCH_T_QUOTED")
	os.Unsetenv("ORCH_T_SINGLE")
	t.Cleanup(func() { os.Unsetenv("ORCH_T_QUOTED"); os.Unsetenv("ORCH_T_SINGLE") })

	if err := loadEnvFile(p); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"ORCH_T_QUOTED": "a b", "ORCH_T_SINGLE": "c=d", "ORCH_T_KEEP": "from-shell", "RAENIL_URL": "",
	} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if err := loadEnvFile(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Errorf("a missing file is not an error: %v", err)
	}
}
