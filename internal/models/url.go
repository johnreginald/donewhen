package models

import (
	"errors"
	"net/url"
	"strings"
)

// MaxURLLen bounds a stored link.
const MaxURLLen = 2048

// ErrInvalidURL means a link field is not an absolute http(s) URL with a host.
var ErrInvalidURL = errors.New("invalid_url: must be an absolute http or https URL")

// ValidateHTTPURL accepts only absolute http(s) URLs with a host. Stored links
// (PR, commit, repo) are rendered as hrefs, so any other scheme — javascript:,
// data:, vbscript:, file: — is a stored-XSS vector and is refused.
func ValidateHTTPURL(raw string) error {
	if raw == "" || len(raw) > MaxURLLen || strings.TrimSpace(raw) != raw {
		return ErrInvalidURL
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return ErrInvalidURL
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return ErrInvalidURL
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return nil
	}
	return ErrInvalidURL
}

// NormalizeURL trims a link field and validates it. An empty value is valid
// and means "clear": it returns nil.
func NormalizeURL(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*raw)
	if v == "" {
		return nil, nil
	}
	if err := ValidateHTTPURL(v); err != nil {
		return nil, err
	}
	return &v, nil
}
