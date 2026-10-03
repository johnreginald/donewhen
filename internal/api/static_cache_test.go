package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The HTML page must be revalidated on every load, or a browser keeps an old build
// for hours after a deploy. Hashed build files may be cached for a year.
func TestServeStaticCacheHeaders(t *testing.T) {
	dir := t.TempDir()
	must := func(p, body string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("index.html", "<html></html>")
	must("sw.js", "//")
	must("_app/immutable/chunks/abc.js", "//")
	s := &Server{staticDir: dir}

	cases := []struct{ path, want string }{
		{"/", "no-cache"},
		{"/issue/DW-1", "no-cache"}, // SPA fallback
		{"/index.html", "no-cache"},
		{"/sw.js", "no-cache"},
		{"/_app/immutable/chunks/abc.js", "public, max-age=31536000, immutable"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		s.serveStatic(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if got := rec.Header().Get("Cache-Control"); got != c.want {
			t.Errorf("%s: Cache-Control = %q, want %q", c.path, got, c.want)
		}
	}
}
