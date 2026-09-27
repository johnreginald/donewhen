package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectEvidenceRunsTheRepoTargetAndUploads(t *testing.T) {
	wt := t.TempDir()
	mk := "evidence:\n\tprintf 'png' > \"$$RAENIL_EVIDENCE_DIR/home.png\"\n\tprintf 'proto' > \"$$RAENIL_EVIDENCE_DIR/home.prototype.png\"\n" +
		"\techo http://preview.local/$$RAENIL_TICKET > \"$$RAENIL_EVIDENCE_DIR/preview.url\"\n\ttouch \"$$RAENIL_EVIDENCE_DIR/ignored.bin\"\n"
	if err := os.WriteFile(filepath.Join(wt, "Makefile"), []byte(mk), 0o644); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/issues/i1/evidence" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Files map[string]string `json:"files"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		got = body.Files
		w.Write([]byte("{}"))
	}))
	defer srv.Close()
	o := &Orchestrator{Raenil: &RaenilClient{BaseURL: srv.URL}, Cfg: Config{Repo: wt}}
	o.CollectEvidence(context.Background(), "i1", "T-1", wt)
	if len(got) != 3 || got["home.png"] == "" || got["home.prototype.png"] == "" || got["preview.url"] == "" {
		t.Errorf("uploaded %v, want the two images and the link, not the .bin", keys(got))
	}
}

func TestCollectEvidenceWithoutTargetDoesNothing(t *testing.T) {
	wt := t.TempDir()
	os.WriteFile(filepath.Join(wt, "Makefile"), []byte("check:\n\ttrue\n"), 0o644)
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	(&Orchestrator{Raenil: &RaenilClient{BaseURL: srv.URL}}).CollectEvidence(context.Background(), "i1", "T-1", wt)
	if called {
		t.Error("uploaded evidence with no evidence target")
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
