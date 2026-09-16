package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RunDir is one attempt's workspace on disk:
//
//	<root>/runs/<ticket>/attempt-<n>/
//	  context.md     what the worker was told
//	  worker.log     raw runner output — never read into a model's context
//	  diff.patch     what the attempt changed
//	  crit-<i>.log   captured output per criterion
//	  evidence.json  one entry per criterion evaluated
//	  verdict.json   the ~20-line summary the judgment plane reads
type RunDir struct {
	Root    string
	Ticket  string
	Attempt int
}

// NewRunDir creates the attempt directory.
func NewRunDir(root, ticket string, attempt int) (*RunDir, error) {
	d := &RunDir{Root: root, Ticket: ticket, Attempt: attempt}
	if err := os.MkdirAll(d.Path(), 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	return d, nil
}

// Path is the attempt directory.
func (d *RunDir) Path() string {
	return filepath.Join(d.Root, "runs", d.Ticket, fmt.Sprintf("attempt-%d", d.Attempt))
}

// File resolves a name inside the attempt directory.
func (d *RunDir) File(name string) string { return filepath.Join(d.Path(), name) }

// WriteEvidence persists the evidence list. Evidence is written as each criterion
// is evaluated, not batched at the end: a dropped session must leave real partial
// progress behind, not an empty file.
func (d *RunDir) WriteEvidence(ev []Evidence) error {
	return writeJSONFile(d.File("evidence.json"), ev)
}

// WriteVerdict persists the verdict.
func (d *RunDir) WriteVerdict(v Verdict) error {
	return writeJSONFile(d.File("verdict.json"), v)
}

// ReadVerdict loads a previously written verdict.
func (d *RunDir) ReadVerdict() (Verdict, error) {
	var v Verdict
	b, err := os.ReadFile(d.File("verdict.json"))
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

// writeJSONFile writes v atomically, so a crash mid-write cannot leave a
// half-written record that later reads as valid.
func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// hashFile returns the hex SHA-256 of a file, so an evidence entry can prove the
// log it points at has not been edited since.
func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
