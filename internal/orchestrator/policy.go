package orchestrator

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// DiffFile is one file touched by an attempt.
type DiffFile struct {
	Path string `json:"path"`
	// Status is one of: added, modified, deleted, renamed.
	Status     string `json:"status"`
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
	// Patch is the unified hunk text for this file, used by content-aware gates.
	Patch string `json:"-"`
}

// Diff is everything an attempt changed.
type Diff struct {
	Files []DiffFile
}

// Stat summarises the diff for a Verdict.
func (d Diff) Stat() DiffStat {
	s := DiffStat{Files: len(d.Files)}
	for _, f := range d.Files {
		s.Insertions += f.Insertions
		s.Deletions += f.Deletions
	}
	return s
}

// addedLines returns the lines a patch introduces, without the leading '+'.
func addedLines(patch string) []string {
	var out []string
	for _, ln := range strings.Split(patch, "\n") {
		if strings.HasPrefix(ln, "+") && !strings.HasPrefix(ln, "+++") {
			out = append(out, ln[1:])
		}
	}
	return out
}

// matchGlob reports whether path p matches a glob supporting '*' (within one
// segment) and '**' (zero or more segments). A trailing "/**" also matches the
// directory itself, so "src/**" covers "src/a.ts" and "src".
func matchGlob(pattern, p string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path.Clean(p), "/"))
}

func matchSegments(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			// '**' at the end swallows whatever is left, including nothing.
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(seg); i++ {
				if matchSegments(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], seg[0])
		if err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}

// matchAny reports whether p matches any of the patterns.
func matchAny(patterns []string, p string) bool {
	for _, g := range patterns {
		if matchGlob(g, p) {
			return true
		}
	}
	return false
}

// testPathPatterns recognises test files across the stacks this repo touches.
var testPathPatterns = []string{
	"**/*_test.go",
	"**/*.test.*", "**/*.spec.*",
	"**/test_*.py", "**/*_test.py",
	"**/tests/**", "**/test/**", "**/__tests__/**",
}

// IsTestPath reports whether a path looks like a test file.
func IsTestPath(p string) bool { return matchAny(testPathPatterns, p) }

// skipMarkers are the ways a worker can neuter a test without deleting it.
var skipMarkers = regexp.MustCompile(
	`(?i)\bt\.Skip\b|\bt\.SkipNow\b|\bdescribe\.skip\b|\bit\.skip\b|\btest\.skip\b|\bxit\b|\bxdescribe\b|@pytest\.mark\.skip|\bunittest\.skip\b|\.only\(`)

// dependencyManifests are files whose change means the dependency set moved.
var dependencyManifests = []string{
	"**/go.mod", "**/go.sum",
	"**/package.json", "**/package-lock.json", "**/pnpm-lock.yaml", "**/yarn.lock", "**/bun.lock", "**/bun.lockb",
	"**/requirements.txt", "**/pyproject.toml", "**/poetry.lock",
	"**/Cargo.toml", "**/Cargo.lock",
}

// secretPatterns catch credentials committed into a diff. Deliberately broad —
// a false positive costs one human glance, a false negative leaks a key.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(aws_secret_access_key|aws_access_key_id)\b\s*[:=]`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{20,}\b`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|secret|passwd|password|token)\b\s*[:=]\s*["'][^"']{12,}["']`),
}

// PolicyResult is the outcome of one gate.
type PolicyResult struct {
	Pass   bool
	Detail string
}

// EvalPolicy runs one named gate against a diff.
//
// Gates are deliberately prevention-shaped: they describe what a well-behaved
// change looks like, so a violation is a refusal rather than a warning.
func EvalPolicy(c PolicyCheck, d Diff) (PolicyResult, error) {
	switch c.Policy {
	case "paths_within":
		if len(c.Args) == 0 {
			return PolicyResult{}, fmt.Errorf("paths_within needs at least one glob")
		}
		var stray []string
		for _, f := range d.Files {
			if !matchAny(c.Args, f.Path) {
				stray = append(stray, f.Path)
			}
		}
		if len(stray) > 0 {
			return PolicyResult{Detail: "touched outside allowed paths: " + strings.Join(stray, ", ")}, nil
		}
		return PolicyResult{Pass: true}, nil

	case "paths_forbidden":
		if len(c.Args) == 0 {
			return PolicyResult{}, fmt.Errorf("paths_forbidden needs at least one glob")
		}
		var hit []string
		for _, f := range d.Files {
			if matchAny(c.Args, f.Path) {
				hit = append(hit, f.Path)
			}
		}
		if len(hit) > 0 {
			return PolicyResult{Detail: "touched forbidden paths: " + strings.Join(hit, ", ")}, nil
		}
		return PolicyResult{Pass: true}, nil

	case "no_new_deps":
		var hit []string
		for _, f := range d.Files {
			if matchAny(dependencyManifests, f.Path) {
				hit = append(hit, f.Path)
			}
		}
		if len(hit) > 0 {
			return PolicyResult{Detail: "dependency manifest changed: " + strings.Join(hit, ", ")}, nil
		}
		return PolicyResult{Pass: true}, nil

	case "no_secrets":
		for _, f := range d.Files {
			for _, ln := range addedLines(f.Patch) {
				for _, re := range secretPatterns {
					if re.MatchString(ln) {
						return PolicyResult{Detail: fmt.Sprintf("possible secret added in %s", f.Path)}, nil
					}
				}
			}
		}
		return PolicyResult{Pass: true}, nil

	case "max_diff_lines":
		if len(c.Args) != 1 {
			return PolicyResult{}, fmt.Errorf("max_diff_lines needs exactly one numeric arg")
		}
		limit, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return PolicyResult{}, fmt.Errorf("max_diff_lines: %q is not a number", c.Args[0])
		}
		st := d.Stat()
		if total := st.Insertions + st.Deletions; total > limit {
			return PolicyResult{Detail: fmt.Sprintf("diff is %d lines, limit %d", total, limit)}, nil
		}
		return PolicyResult{Pass: true}, nil

	case "tests_not_weakened":
		// The reward hack this whole design exists to stop: making the suite green
		// by deleting or skipping the test rather than fixing the code.
		for _, f := range d.Files {
			if !IsTestPath(f.Path) {
				continue
			}
			if f.Status == "deleted" {
				return PolicyResult{Detail: "test file deleted: " + f.Path}, nil
			}
			for _, ln := range addedLines(f.Patch) {
				if skipMarkers.MatchString(ln) {
					return PolicyResult{Detail: fmt.Sprintf("test skip/only marker added in %s: %s",
						f.Path, strings.TrimSpace(ln))}, nil
				}
			}
		}
		return PolicyResult{Pass: true}, nil

	default:
		return PolicyResult{}, fmt.Errorf("unknown policy %q", c.Policy)
	}
}
