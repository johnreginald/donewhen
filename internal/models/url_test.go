package models

import "testing"

func TestValidateHTTPURL(t *testing.T) {
	good := []string{
		"https://github.com/x/y/pull/1",
		"http://localhost:3000/a?b=c#d",
		"HTTPS://Example.com/x",
	}
	for _, u := range good {
		if err := ValidateHTTPURL(u); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	bad := []string{
		"", " ", "javascript:alert(1)", "JaVaScRiPt:alert(1)", "data:text/html,<script>1</script>",
		"vbscript:x", "file:///etc/passwd", "ftp://example.com/x", "//example.com/x", "/relative/path",
		"github.com/x/y", "https://", "http:///nohost", "https://exa mple.com", " https://example.com",
		"https://example.com/\n", "javascript://example.com/%0aalert(1)",
	}
	for _, u := range bad {
		if err := ValidateHTTPURL(u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	if got, err := NormalizeURL(nil); got != nil || err != nil {
		t.Errorf("nil -> %v, %v", got, err)
	}
	empty := "  "
	if got, err := NormalizeURL(&empty); got != nil || err != nil {
		t.Errorf("blank should clear: %v, %v", got, err)
	}
	v := " https://a.b/c "
	if got, err := NormalizeURL(&v); err != nil || got == nil || *got != "https://a.b/c" {
		t.Errorf("trim failed: %v, %v", got, err)
	}
	bad := "javascript:alert(1)"
	if _, err := NormalizeURL(&bad); err == nil {
		t.Error("bad scheme accepted")
	}
}
