package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"raenil/internal/models"
)

func TestInspectRepoReadsTheBuildSurface(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"),
		[]byte(`{"scripts":{"build":"astro build","dev":"astro dev"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("build: ## x\n\tgo build\n\ntest:\n\tgo test\n"), 0o644)

	f, err := InspectRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Scripts["build"] != "astro build" {
		t.Errorf("scripts = %v", f.Scripts)
	}
	if len(f.MakeTargets) < 2 {
		t.Errorf("make targets = %v", f.MakeTargets)
	}
	s := f.String()
	// The two facts that most often make a model invent a wrong command.
	for _, want := range []string{"no test script", "node_modules", "package-lock.json"} {
		if !strings.Contains(s, want) {
			t.Errorf("facts should warn about %q:\n%s", want, s)
		}
	}
	if !strings.Contains(s, "NO .gitignore") {
		t.Errorf("facts should warn about a missing .gitignore:\n%s", s)
	}
}

func TestExtractJSONArrayFromMessyReplies(t *testing.T) {
	want := `[{"text":"x"}]`
	for _, in := range []string{
		want,
		"Here you go:\n```json\n" + want + "\n```\n",
		"```\n" + want + "\n```",
		"Sure — " + want + " — hope that helps",
	} {
		got, err := extractJSONArray(in)
		if err != nil {
			t.Errorf("input %q: %v", in, err)
			continue
		}
		if strings.ReplaceAll(string(got), " ", "") != want {
			t.Errorf("input %q gave %q", in, got)
		}
	}
	if _, err := extractJSONArray("I could not do that"); err == nil {
		t.Error("prose with no array should error")
	}
}

func TestProposeRejectsUngatingDrafts(t *testing.T) {
	issue := models.Issue{Key: "T-1", Title: "x"}
	facts := RepoFacts{Root: "/tmp"}

	// All-manual: nothing an orchestrator could decide.
	a := &stubAsker{answer: `[{"text":"looks nice","kind":"manual"}]`}
	if _, _, err := Propose(context.Background(), a, "m", issue, facts); err == nil ||
		!strings.Contains(err.Error(), "machine-checkable") {
		t.Errorf("expected an all-manual draft to be refused, got %v", err)
	}

	// A deterministic criterion with no command cannot gate either.
	a = &stubAsker{answer: `[{"text":"builds","kind":"deterministic","check":{}}]`}
	if _, _, err := Propose(context.Background(), a, "m", issue, facts); err == nil {
		t.Error("expected a check with no cmd to be refused")
	}

	// Prose instead of JSON.
	a = &stubAsker{answer: `I would suggest making sure the build passes.`}
	if _, _, err := Propose(context.Background(), a, "m", issue, facts); err == nil {
		t.Error("expected a non-JSON reply to be refused")
	}
}

func TestProposeAcceptsAUsableDraft(t *testing.T) {
	a := &stubAsker{answer: "```json\n" + `[
	  {"text":"build passes","kind":"deterministic","check":{"cmd":"npm run build","expect_exit":0}},
	  {"text":"scoped","kind":"policy","check":{"policy":"paths_within","args":["src/**"]}}
	]` + "\n```"}
	items, _, err := Propose(context.Background(), a, "m", models.Issue{Key: "T-1"}, RepoFacts{})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(items) != 2 || items[0].Kind != models.CriterionDeterministic {
		t.Errorf("items = %+v", items)
	}
	// The prompt must carry the repo's real commands, or the model guesses.
	if !strings.Contains(a.gotPrompt, "Do not invent a test runner") {
		t.Error("prompt should forbid inventing commands")
	}
}

// A criterion that already passes waves work through. Saying so is the point.
func TestVerifyCriteriaFailTodayFlagsAlreadyPassing(t *testing.T) {
	dir := t.TempDir()
	items := []ProposedCriterion{
		{Text: "always true", Kind: models.CriterionDeterministic, Check: []byte(`{"cmd":"true","timeout":"30s"}`)},
		{Text: "always false", Kind: models.CriterionDeterministic, Check: []byte(`{"cmd":"false","timeout":"30s"}`)},
		{Text: "scoped", Kind: models.CriterionPolicy, Check: []byte(`{"policy":"paths_within","args":["**"]}`)},
	}
	out, err := VerifyCriteriaFailToday(context.Background(), dir, items)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "ALREADY PASSES") {
		t.Errorf("a passing check must be called out:\n%s", joined)
	}
	if !strings.Contains(joined, "fails today") {
		t.Errorf("a failing check should be reported as correct:\n%s", joined)
	}
}
