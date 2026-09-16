package orchestrator

import "testing"

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"src/**", "src/a.ts", true},
		{"src/**", "src/deep/nested/a.ts", true},
		{"src/**", "src", true},
		{"src/**", "lib/a.ts", false},
		{"src/*", "src/a.ts", true},
		{"src/*", "src/deep/a.ts", false},
		{"**/*_test.go", "internal/store/dev_test.go", true},
		{"**/*_test.go", "dev_test.go", true},
		{"**/*_test.go", "internal/store/dev.go", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false},
		{"**/tests/**", "app/tests/unit/a.py", true},
		{"**/tests/**", "app/testing/a.py", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestPathsWithin(t *testing.T) {
	d := Diff{Files: []DiffFile{
		{Path: "src/index.ts", Status: "modified"},
		{Path: "src/lib/util.ts", Status: "added"},
	}}
	res, err := EvalPolicy(PolicyCheck{Policy: "paths_within", Args: []string{"src/**"}}, d)
	if err != nil || !res.Pass {
		t.Fatalf("expected pass, got pass=%v err=%v detail=%q", res.Pass, err, res.Detail)
	}

	d.Files = append(d.Files, DiffFile{Path: ".github/workflows/deploy.yml", Status: "modified"})
	res, _ = EvalPolicy(PolicyCheck{Policy: "paths_within", Args: []string{"src/**"}}, d)
	if res.Pass {
		t.Error("expected failure when a file escapes the allowed globs")
	}
}

func TestNoNewDeps(t *testing.T) {
	clean := Diff{Files: []DiffFile{{Path: "src/a.ts"}}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "no_new_deps"}, clean); !res.Pass {
		t.Error("clean diff should pass no_new_deps")
	}
	dirty := Diff{Files: []DiffFile{{Path: "package.json"}}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "no_new_deps"}, dirty); res.Pass {
		t.Error("package.json change should fail no_new_deps")
	}
}

func TestMaxDiffLines(t *testing.T) {
	d := Diff{Files: []DiffFile{{Path: "a.ts", Insertions: 60, Deletions: 30}}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "max_diff_lines", Args: []string{"100"}}, d); !res.Pass {
		t.Error("90 lines should pass a limit of 100")
	}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "max_diff_lines", Args: []string{"50"}}, d); res.Pass {
		t.Error("90 lines should fail a limit of 50")
	}
	if _, err := EvalPolicy(PolicyCheck{Policy: "max_diff_lines", Args: []string{"lots"}}, d); err == nil {
		t.Error("non-numeric limit should error")
	}
}

func TestNoSecrets(t *testing.T) {
	safe := Diff{Files: []DiffFile{{Path: "a.ts", Patch: "+const name = \"hello\"\n-old line"}}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "no_secrets"}, safe); !res.Pass {
		t.Error("ordinary code should pass no_secrets")
	}
	for _, leak := range []string{
		`+AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE`,
		`+const key = "sk-abcdefghijklmnopqrstuvwxyz"`,
		`+-----BEGIN RSA PRIVATE KEY-----`,
		`+password = "hunter2-hunter2-hunter2"`,
	} {
		d := Diff{Files: []DiffFile{{Path: "a.ts", Patch: leak}}}
		if res, _ := EvalPolicy(PolicyCheck{Policy: "no_secrets"}, d); res.Pass {
			t.Errorf("expected no_secrets to catch %q", leak)
		}
	}
	// A removed secret must not trip the gate — that is a fix, not a leak.
	removed := Diff{Files: []DiffFile{{Path: "a.ts", Patch: `-const key = "sk-abcdefghijklmnopqrstuvwxyz"`}}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "no_secrets"}, removed); !res.Pass {
		t.Error("removing a secret should pass no_secrets")
	}
}

func TestTestsNotWeakened(t *testing.T) {
	ok := Diff{Files: []DiffFile{
		{Path: "src/add.ts", Status: "modified", Patch: "+return a + b"},
		{Path: "src/add.test.ts", Status: "modified", Patch: "+expect(add(2,3)).toBe(5)"},
	}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "tests_not_weakened"}, ok); !res.Pass {
		t.Error("adding a real assertion should pass")
	}

	deleted := Diff{Files: []DiffFile{{Path: "internal/store/dev_test.go", Status: "deleted"}}}
	res, _ := EvalPolicy(PolicyCheck{Policy: "tests_not_weakened"}, deleted)
	if res.Pass {
		t.Error("deleting a test file must fail the gate")
	}

	for _, marker := range []string{
		"+	t.Skip(\"flaky\")",
		"+  it.skip('adds', () => {})",
		"+  describe.skip('suite', () => {})",
		"+@pytest.mark.skip",
		"+  it.only('just this one', () => {})",
	} {
		d := Diff{Files: []DiffFile{{Path: "src/add.test.ts", Status: "modified", Patch: marker}}}
		if res, _ := EvalPolicy(PolicyCheck{Policy: "tests_not_weakened"}, d); res.Pass {
			t.Errorf("expected skip marker to fail the gate: %q", marker)
		}
	}

	// A skip added to non-test code is not this gate's business.
	nonTest := Diff{Files: []DiffFile{{Path: "src/app.ts", Status: "modified", Patch: "+// t.Skip is mentioned here"}}}
	if res, _ := EvalPolicy(PolicyCheck{Policy: "tests_not_weakened"}, nonTest); !res.Pass {
		t.Error("skip marker outside a test file should not fail the gate")
	}
}

func TestUnknownPolicyErrors(t *testing.T) {
	if _, err := EvalPolicy(PolicyCheck{Policy: "vibes"}, Diff{}); err == nil {
		t.Error("unknown policy must error rather than silently pass")
	}
}
