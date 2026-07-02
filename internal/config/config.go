// Package config loads Kanri configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	BaseURL       string // public URL, e.g. https://tracker.mydomain
	ListenAddr    string // e.g. :8080
	DatabaseURL   string
	Env           string // dev | prod
	SessionSecret string
	IssuePrefix   string

	VAPIDPublic  string
	VAPIDPrivate string
	VAPIDSubject string
}

func (c Config) IsProd() bool { return c.Env == "prod" }

// SecureCookies returns true when cookies must carry the Secure flag.
// Prod always; dev only when serving over https.
func (c Config) SecureCookies() bool {
	return c.IsProd() || strings.HasPrefix(c.BaseURL, "https://")
}

// PushEnabled reports whether Web Push is configured.
func (c Config) PushEnabled() bool {
	return c.VAPIDPublic != "" && c.VAPIDPrivate != ""
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment, applying defaults.
func Load() (Config, error) {
	c := Config{
		BaseURL:       env("KANRI_BASE_URL", "http://localhost:8080"),
		ListenAddr:    env("KANRI_LISTEN_ADDR", ":8080"),
		DatabaseURL:   env("KANRI_DATABASE_URL", "postgres://kanri:kanri@localhost:5432/kanri?sslmode=disable"),
		Env:           env("KANRI_ENV", "dev"),
		SessionSecret: os.Getenv("KANRI_SESSION_SECRET"),
		IssuePrefix:   env("KANRI_ISSUE_PREFIX", "K"),
		VAPIDPublic:   os.Getenv("KANRI_VAPID_PUBLIC"),
		VAPIDPrivate:  os.Getenv("KANRI_VAPID_PRIVATE"),
		VAPIDSubject:  env("KANRI_VAPID_SUBJECT", "mailto:admin@localhost"),
	}
	if c.SessionSecret == "" {
		if c.IsProd() {
			return c, fmt.Errorf("KANRI_SESSION_SECRET is required in prod")
		}
		c.SessionSecret = "dev-insecure-session-secret-do-not-use-in-prod"
	}
	if len(c.SessionSecret) < 16 {
		return c, fmt.Errorf("KANRI_SESSION_SECRET must be at least 16 bytes")
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	return c, nil
}
