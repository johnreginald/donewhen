package config

import "testing"

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"DONEWHEN_ENV", "DONEWHEN_SESSION_SECRET"} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoadEnvHandling(t *testing.T) {
	const good = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
		prod    bool
	}{
		{"default env is dev with dev secret", nil, false, false},
		{"explicit dev", map[string]string{"DONEWHEN_ENV": "dev"}, false, false},
		{"prod without secret", map[string]string{"DONEWHEN_ENV": "prod"}, true, true},
		{"production without secret", map[string]string{"DONEWHEN_ENV": "production"}, true, true},
		{"typo without secret", map[string]string{"DONEWHEN_ENV": "prodd"}, true, true},
		{"prod with real secret (legacy)", map[string]string{"DONEWHEN_ENV": "prod", "DONEWHEN_SESSION_SECRET": good}, false, true},
		{"production with real secret", map[string]string{"DONEWHEN_ENV": "production", "DONEWHEN_SESSION_SECRET": good}, false, true},
		{"prod with the public dev secret", map[string]string{"DONEWHEN_ENV": "prod", "DONEWHEN_SESSION_SECRET": DevSessionSecret}, true, true},
		{"uppercase PROD is still prod", map[string]string{"DONEWHEN_ENV": "PROD", "DONEWHEN_SESSION_SECRET": good}, false, true},
		{"dev with short secret", map[string]string{"DONEWHEN_ENV": "dev", "DONEWHEN_SESSION_SECRET": "short"}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			c, err := Load()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && c.IsProd() != tc.prod {
				t.Fatalf("IsProd = %v, want %v", c.IsProd(), tc.prod)
			}
		})
	}
}

func TestLoadAcceptsLegacyRaenilNames(t *testing.T) {
	setEnv(t, nil)
	t.Setenv("RAENIL_ISSUE_PREFIX", "OLD")
	t.Setenv("DONEWHEN_ISSUE_PREFIX", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.IssuePrefix != "OLD" {
		t.Fatalf("IssuePrefix = %q, want OLD (from RAENIL_ISSUE_PREFIX)", c.IssuePrefix)
	}
	t.Setenv("DONEWHEN_ISSUE_PREFIX", "NEW")
	if c, _ = Load(); c.IssuePrefix != "NEW" {
		t.Fatalf("IssuePrefix = %q, want NEW (new name wins)", c.IssuePrefix)
	}
}
