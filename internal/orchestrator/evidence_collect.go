package orchestrator

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Evidence is what a change looks like, for the user's review: screenshots
// of each screen state — with the prototype screen beside each, when named
// <name>.prototype.png — and a preview link in preview.url.
//
// The repository makes it: when its Makefile has an `evidence` target, the
// runner host runs `make evidence` with RAENIL_EVIDENCE_DIR set, after the
// checks pass, and uploads what lands there. Raenil knows nothing about how a
// project takes screenshots; the project does.

// EvidenceTimeout bounds `make evidence`.
const EvidenceTimeout = 10 * time.Minute

var evidenceTarget = regexp.MustCompile(`(?m)^evidence\s*:`)

// hasEvidenceTarget reports whether the worktree's Makefile can make evidence.
func hasEvidenceTarget(wtPath string) bool {
	b, err := os.ReadFile(filepath.Join(wtPath, "Makefile"))
	return err == nil && evidenceTarget.Match(b)
}

var evidenceExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".url": true, ".txt": true, ".md": true}

// CollectEvidence runs the repository's `make evidence` in the worktree and
// uploads the files to the ticket. A repository without the target has no
// evidence; a failing run is logged, never a reason to hold the ticket back.
func (o *Orchestrator) CollectEvidence(ctx context.Context, issueID, issueKey, wtPath string) {
	if !hasEvidenceTarget(wtPath) {
		return
	}
	dir, err := os.MkdirTemp("", "raenil-evidence-")
	if err != nil {
		return
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, EvidenceTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "make", "evidence")
	cmd.Dir = wtPath
	cmd.Env = append(append(os.Environ(), repoEnv(o.Cfg.withDefaults().Repo)...),
		"RAENIL_EVIDENCE_DIR="+dir, "RAENIL_TICKET="+issueKey)
	cmd.Stdin = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		o.logf("warning: make evidence failed: %v: %s", err, tailString(string(out), 400))
		return
	}
	files := map[string]string{}
	entries, _ := os.ReadDir(dir)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !evidenceExt[strings.ToLower(filepath.Ext(e.Name()))] || len(files) >= 40 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || len(b) > 8<<20 {
			continue
		}
		files[e.Name()] = base64.StdEncoding.EncodeToString(b)
	}
	if len(files) == 0 {
		o.logf("make evidence produced nothing")
		return
	}
	if err := o.Raenil.do(ctx, http.MethodPut, "/api/issues/"+issueID+"/evidence", map[string]any{"files": files}, nil); err != nil {
		o.logf("warning: could not upload the evidence: %v", err)
		return
	}
	o.logf("evidence: %d file(s)", len(files))
}

func tailString(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}
