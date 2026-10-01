package config

import "testing"

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"RAENIL_ENV", "RAENIL_SESSION_SECRET"} {
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
		{"explicit dev", map[string]string{"RAENIL_ENV": "dev"}, false, false},
		{"prod without secret", map[string]string{"RAENIL_ENV": "prod"}, true, true},
		{"production without secret", map[string]string{"RAENIL_ENV": "production"}, true, true},
		{"typo without secret", map[string]string{"RAENIL_ENV": "prodd"}, true, true},
		{"prod with real secret (legacy)", map[string]string{"RAENIL_ENV": "prod", "RAENIL_SESSION_SECRET": good}, false, true},
		{"production with real secret", map[string]string{"RAENIL_ENV": "production", "RAENIL_SESSION_SECRET": good}, false, true},
		{"prod with the public dev secret", map[string]string{"RAENIL_ENV": "prod", "RAENIL_SESSION_SECRET": DevSessionSecret}, true, true},
		{"uppercase PROD is still prod", map[string]string{"RAENIL_ENV": "PROD", "RAENIL_SESSION_SECRET": good}, false, true},
		{"dev with short secret", map[string]string{"RAENIL_ENV": "dev", "RAENIL_SESSION_SECRET": "short"}, true, false},
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
