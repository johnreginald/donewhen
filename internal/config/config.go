// Package config loads Raenil configuration from environment variables.
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
	// TrustedProxyHeader names the request header a trusted reverse proxy sets
	// to the real client address (CF-Connecting-IP, X-Forwarded-For). Empty
	// means no proxy is trusted and the TCP peer address is used.
	TrustedProxyHeader string

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
		BaseURL:            env("RAENIL_BASE_URL", "http://localhost:8080"),
		ListenAddr:         env("RAENIL_LISTEN_ADDR", ":8080"),
		DatabaseURL:        env("RAENIL_DATABASE_URL", "postgres://raenil:raenil@localhost:5432/raenil?sslmode=disable"),
		Env:                env("RAENIL_ENV", "dev"),
		SessionSecret:      os.Getenv("RAENIL_SESSION_SECRET"),
		IssuePrefix:        env("RAENIL_ISSUE_PREFIX", "R"),
		TrustedProxyHeader: strings.TrimSpace(os.Getenv("RAENIL_TRUSTED_PROXY_HEADER")),
		VAPIDPublic:        os.Getenv("RAENIL_VAPID_PUBLIC"),
		VAPIDPrivate:       os.Getenv("RAENIL_VAPID_PRIVATE"),
		VAPIDSubject:       env("RAENIL_VAPID_SUBJECT", "mailto:admin@localhost"),
	}
	if c.SessionSecret == "" {
		if c.IsProd() {
			return c, fmt.Errorf("RAENIL_SESSION_SECRET is required in prod")
		}
		c.SessionSecret = "dev-insecure-session-secret-do-not-use-in-prod"
	}
	if len(c.SessionSecret) < 16 {
		return c, fmt.Errorf("RAENIL_SESSION_SECRET must be at least 16 bytes")
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	return c, nil
}
