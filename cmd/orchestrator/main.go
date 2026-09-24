// Command orchestrator drives a Raenil ticket to a verdict: it claims the
// ticket, runs a coding agent in an isolated worktree, and decides from evidence
// whether the work satisfies the ticket's done-when criteria.
//
// It is the control plane. No model it calls gets to say whether the work is
// finished — a command exits zero or it does not.
//
//	orchestrator health
//	orchestrator run  BD-12 --repo ~/Project/my-project
//	orchestrator check BD-12 --repo ~/Project/my-project
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"raenil/internal/models"
	"raenil/internal/orchestrator"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	// Ctrl-C must unwind cleanly: a killed run still has to release its worktree.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "run":
		err = cmdRun(ctx, args, false)
	case "check":
		err = cmdRun(ctx, args, true)
	case "work":
		err = cmdWork(ctx, args)
	case "daemon":
		err = cmdDaemon(ctx, args)
	case "host":
		err = cmdHost(ctx, args)
	case "status":
		err = cmdStatus(args)
	case "bench":
		err = cmdBench(ctx, args)
	case "propose":
		err = cmdPropose(ctx, args)
	case "verify":
		err = cmdReview(ctx, args, false)
	case "finish":
		err = cmdReview(ctx, args, true)
	case "health":
		err = cmdHealth(ctx)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `orchestrator — drive a Raenil ticket to a verdict

  orchestrator health                 check the tracker and the agent are reachable
  orchestrator run   <TICKET>         one attempt: claim, run an agent, evaluate, record
  orchestrator work  <TICKET>         attempt, triage, retry/escalate, or hand back
  orchestrator check <TICKET>         evaluate the criteria against the repo as it
                                      stands, without running an agent
  orchestrator propose <TICKET>       draft a typed done-when checklist for a ticket
  orchestrator verify  <TICKET>       re-run the criteria against the kept worktree
  orchestrator finish  <TICKET>       commit review fixes, record, move to In Review
  orchestrator daemon                 work the Ready queue unattended
  orchestrator host                   serve Raenil's web UI from this machine: report
                                      which agents can run here, and run the work
                                      queued from the dashboard
  orchestrator status                 show held leases and the kill switch
  orchestrator bench                  compare models over the fixture tasks

Flags (daemon only):
  --queue STATE      state to pull from (default: Ready)
  --runner NAME      default worker: opencode, codex or claude (default: opencode)
  --watch            subscribe to Raenil's event stream; a ticket entering the
                     queue state is picked up at once instead of on the next sweep
  --poll DUR         reconcile sweep interval (default 30s, or 5m with --watch).
                     The sweep is what makes a dropped event cost latency only.
  --concurrency N    tickets in flight (default: 1 — see docs/ORCHESTRATOR.md)
  --max-cost-hour U  halt once spend in a rolling hour exceeds this
  --stop-file PATH   kill switch; the daemon halts while it exists
  --require-label L  only take issues carrying this label (e.g. ready-for-agent)

Flags (work/verify/finish/propose):
  --runner NAME      default worker: opencode, codex or claude. A runner: label on the
                     ticket overrides it, so a ticket can pick its own agent.
  --asker NAME       who answers judgment criteria and drafts checklists

Flags (run/work):
  --handoff          stop after the worker: commit its work, keep the worktree,
                     and leave the ticket alone so a reviewer takes it from there

Flags (work only):
  --max-attempts N   hard stop before handing back to a human (default: 3)
  --escalate-after N attempt after which to use the stronger model (default: 2)
  --escalate-model M the stronger model (default: $ORCHESTRATOR_ESCALATE_MODEL)
  --max-cost USD     stop once a ticket has cost this much across attempts
  --judge-model M    model for judgment criteria (default: the escalate model)

Flags (run/check/work):
  --repo PATH        repository to work in (default: current directory)
  --repos LIST       route by repo: label — api-mobile=/p/a,api-server=/p/b
                     A ticket naming a repo with no path here is refused, never
                     worked in the wrong one. ($ORCHESTRATOR_REPOS)
  --run-root PATH    where run dirs live (default: .orchestrator)
  --base REF         what the worktree branches from (default: HEAD)
  --model NAME       provider/model for the worker (default: $ORCHESTRATOR_MODEL)
  --attempt N        attempt number (default: 1)
  --timeout DUR      bound one attempt (default: 30m)
  --json             print the verdict as JSON

Environment:
  RAENIL_URL         tracker base URL, e.g. https://tracker.example.com
  RAENIL_TOKEN       API token (raenil token <name>)
  RAENIL_WORKSPACE   pin every call to one workspace. Leave unset with a token
                     that spans several: the workspace is then taken from the
                     ticket key, so API-42 finds its own project.
  ORCHESTRATOR_REPOS default --repos routing
  OPENCODE_URL       running 'opencode serve', e.g. http://127.0.0.1:4096
  ORCHESTRATOR_MODEL default worker model, e.g. opencode-go/glm-5.3-flash
  ORCHESTRATOR_JUDGE_MODEL     model for judgment criteria (falls back to the escalate model)
  ORCHESTRATOR_PROPOSE_MODEL   model for drafting criteria (falls back to the judge model)
  ORCHESTRATOR_ESCALATE_MODEL  stronger model for escalation — must beat the worker
                               model, or escalating achieves nothing
`)
}

// clients builds the tracker and agent clients from the environment, failing
// with an actionable message rather than a nil-pointer panic later.
func clients() (*orchestrator.RaenilClient, *orchestrator.OpenCodeRunner, error) {
	base := strings.TrimRight(os.Getenv("RAENIL_URL"), "/")
	token := os.Getenv("RAENIL_TOKEN")
	if base == "" || token == "" {
		return nil, nil, fmt.Errorf("RAENIL_URL and RAENIL_TOKEN must be set")
	}
	rc := &orchestrator.RaenilClient{
		BaseURL:   base,
		Token:     token,
		Workspace: os.Getenv("RAENIL_WORKSPACE"),
	}
	oc := &orchestrator.OpenCodeRunner{
		BaseURL: strings.TrimRight(os.Getenv("OPENCODE_URL"), "/"),
	}
	return rc, oc, nil
}

// parsePermuted parses flags that may appear before OR after positional
// arguments. Go's flag package stops at the first non-flag token, so
// `work PP-175 --repo X` would silently ignore --repo and use its default —
// which is how a ticket ends up being worked in the wrong repository.
func parsePermuted(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
}

// parseRepos reads "name=path,name=path" into a repo set. Paths are checked now
// rather than when a ticket needs one, so a typo surfaces at startup instead of
// halfway through a run.
func parseRepos(spec string) (orchestrator.RepoSet, error) {
	set := orchestrator.RepoSet{}
	for _, pair := range strings.Split(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, path, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("--repos entry %q should look like name=/path", pair)
		}
		name, path = strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(path)
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			return nil, fmt.Errorf("--repos %s: %s is not a git repository", name, path)
		}
		set[name] = path
	}
	return set, nil
}

// runnerPool builds every runner this machine can drive. A ticket picks one with
// a runner: label; anything unlabelled uses the default.
func runnerPool(ctx context.Context, oc *orchestrator.OpenCodeRunner) orchestrator.RunnerSet {
	set := orchestrator.RunnerSet{}
	if oc != nil && oc.BaseURL != "" {
		set["opencode"] = oc
	}
	cx := &orchestrator.CodexRunner{}
	if err := cx.Available(ctx); err == nil {
		set["codex"] = cx
	}
	cl := claudeRunner()
	if err := cl.Available(ctx); err == nil {
		set["claude"] = cl
	}
	return set
}

// claudeRunner builds the Claude Code runner. CLAUDE_ALLOWED_TOOLS lists the
// extra commands a worker may run without asking, as Claude Code permission
// rules separated by ";" — e.g. "Bash(go test *);Bash(go build *)". Bare Bash is
// never granted: it would approve writes anywhere on the machine.
func claudeRunner() *orchestrator.ClaudeRunner {
	var allowed []string
	for _, rule := range strings.Split(os.Getenv("CLAUDE_ALLOWED_TOOLS"), ";") {
		if rule = strings.TrimSpace(rule); rule != "" {
			allowed = append(allowed, rule)
		}
	}
	return &orchestrator.ClaudeRunner{AllowedTools: allowed}
}

// pickAsker chooses who answers judgment criteria and drafts checklists.
func pickAsker(name string, oc *orchestrator.OpenCodeRunner) (orchestrator.Asker, error) {
	switch strings.ToLower(name) {
	case "", "opencode":
		if oc == nil || oc.BaseURL == "" {
			return nil, fmt.Errorf("OPENCODE_URL must be set to use the opencode asker")
		}
		return oc, nil
	case "codex":
		return &orchestrator.CodexRunner{}, nil
	case "claude":
		return claudeRunner(), nil
	default:
		return nil, fmt.Errorf("unknown asker %q (opencode, codex, claude)", name)
	}
}

func cmdHealth(ctx context.Context) error {
	rc, oc, err := clients()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	ok := true
	if err := rc.Health(ctx); err != nil {
		fmt.Printf("raenil    %-10s %s\n", "DOWN", err)
		ok = false
	} else {
		fmt.Printf("raenil    %-10s %s\n", "ok", rc.BaseURL)
	}
	if oc.BaseURL == "" {
		fmt.Printf("opencode  %-10s OPENCODE_URL not set\n", "SKIP")
	} else if v, err := oc.Health(ctx); err != nil {
		fmt.Printf("opencode  %-10s %s\n", "DOWN", err)
		ok = false
	} else {
		fmt.Printf("opencode  %-10s %s (%s)\n", "ok", oc.BaseURL, v)
	}
	cx := &orchestrator.CodexRunner{}
	if err := cx.Available(ctx); err != nil {
		fmt.Printf("codex     %-10s %s\n", "SKIP", err)
	} else {
		fmt.Printf("codex     %-10s installed and logged in\n", "ok")
	}

	if !ok {
		return fmt.Errorf("one or more dependencies are unreachable")
	}
	return nil
}

func cmdRun(ctx context.Context, args []string, checkOnly bool) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository to work in")
	reposSpec := fs.String("repos", os.Getenv("ORCHESTRATOR_REPOS"), "repo:label routing, e.g. api-mobile=/p/a,api-server=/p/b")
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branches from")
	model := fs.String("model", os.Getenv("ORCHESTRATOR_MODEL"), "provider/model for the worker")
	attempt := fs.Int("attempt", 1, "attempt number")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	asJSON := fs.Bool("json", false, "print the verdict as JSON")
	judgeModel := fs.String("judge-model", os.Getenv("ORCHESTRATOR_JUDGE_MODEL"), "model for judgment criteria")
	handoff := fs.Bool("handoff", false, "stop after the worker: commit its work, keep the worktree, leave the ticket alone")
	runnerName := fs.String("runner", "opencode", "default worker: opencode, codex or claude (a runner: label on the ticket wins)")
	askerName := fs.String("asker", "opencode", "who answers judgment criteria: opencode, codex or claude")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("a ticket key is required, e.g. BD-12")
	}
	ticket := pos[0]

	rc, oc, cerr := clients()
	if cerr != nil {
		return cerr
	}
	if !checkOnly {
		// Only the OpenCode runner needs its server; Codex and Claude are CLIs.
		if oc.BaseURL == "" && strings.EqualFold(*runnerName, "opencode") {
			return fmt.Errorf("OPENCODE_URL must be set to run an agent on opencode (use --runner codex|claude, or 'check' to evaluate without one)")
		}
		if *model == "" {
			return fmt.Errorf("a model is required: --model or ORCHESTRATOR_MODEL " +
				"(needs a provider prefix, e.g. opencode-go/glm-5.3-flash)")
		}
	}

	var runner orchestrator.Runner = oc
	pool := runnerPool(ctx, oc)
	if r, ok := pool[strings.ToLower(*runnerName)]; ok {
		runner = r
	} else if *runnerName != "opencode" {
		return fmt.Errorf("runner %q is not available (have: %s)", *runnerName, strings.Join(pool.Names(), ", "))
	}
	if checkOnly {
		runner = noopRunner{}
	}

	o := &orchestrator.Orchestrator{
		Raenil: rc,
		Runner: runner,
		Cfg: orchestrator.Config{
			RunRoot: *runRoot,
			Repo:    *repo,
			BaseRef: *baseRef,
			Model:   *model,
			Timeout: *timeout,
			Handoff: *handoff,
		},
		Log: func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	}
	repos, rerr := parseRepos(*reposSpec)
	if rerr != nil {
		return rerr
	}
	o.Repos = repos
	if *judgeModel != "" {
		asker, aerr := pickAsker(*askerName, oc)
		if aerr != nil {
			return aerr
		}
		o.Judge = orchestrator.NewJudge(asker, *judgeModel)
	}

	v, err := o.RunTicket(ctx, ticket, *attempt)
	if err != nil {
		return err
	}
	if *asJSON {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	}
	// A failing ticket is a result, not a crash — but the exit code has to say so
	// for a shell or a CI step to react.
	if v.Status != orchestrator.StatusPassed {
		os.Exit(3)
	}
	return nil
}

func cmdWork(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("work", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository to work in")
	reposSpec := fs.String("repos", os.Getenv("ORCHESTRATOR_REPOS"), "repo:label routing, e.g. api-mobile=/p/a,api-server=/p/b")
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branches from")
	model := fs.String("model", os.Getenv("ORCHESTRATOR_MODEL"), "provider/model for the worker")
	escalateModel := fs.String("escalate-model", os.Getenv("ORCHESTRATOR_ESCALATE_MODEL"), "stronger model")
	judgeModel := fs.String("judge-model", os.Getenv("ORCHESTRATOR_JUDGE_MODEL"), "model for judgment criteria")
	maxAttempts := fs.Int("max-attempts", 3, "hard stop before handing back")
	escalateAfter := fs.Int("escalate-after", 2, "attempt after which to escalate")
	maxCost := fs.Float64("max-cost", 0, "stop once a ticket has cost this much")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	asJSON := fs.Bool("json", false, "print the verdict as JSON")
	handoff := fs.Bool("handoff", false, "stop after the worker: commit its work, keep the worktree, leave the ticket alone")
	runnerName := fs.String("runner", "opencode", "default worker: opencode, codex or claude (a runner: label on the ticket wins)")
	askerName := fs.String("asker", "opencode", "who answers judgment criteria: opencode, codex or claude")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("a ticket key is required, e.g. BD-12")
	}
	ticket := pos[0]

	rc, oc, err := clients()
	if err != nil {
		return err
	}
	// Only the OpenCode runner needs its server; Codex and Claude are CLIs.
	if oc.BaseURL == "" && strings.EqualFold(*runnerName, "opencode") {
		return fmt.Errorf("OPENCODE_URL must be set to work on opencode (or use --runner codex|claude)")
	}
	if *model == "" {
		return fmt.Errorf("a model is required: --model or ORCHESTRATOR_MODEL")
	}

	jm := *judgeModel
	if jm == "" {
		jm = *escalateModel
	}
	o := &orchestrator.Orchestrator{
		Raenil: rc,
		Runner: oc,
		Cfg: orchestrator.Config{
			RunRoot: *runRoot, Repo: *repo, BaseRef: *baseRef,
			Model: *model, Timeout: *timeout, Handoff: *handoff,
		},
		// Claim the ticket for the duration of the run. Without this, two
		// terminals working the same ticket each cut a worktree and a branch and
		// race to commit — and this hand-run path is the common one.
		Leases: &orchestrator.LeaseManager{Dir: filepath.Join(*runRoot, "leases")},
		Log:    func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	}
	if repos, rerr := parseRepos(*reposSpec); rerr != nil {
		return rerr
	} else {
		o.Repos = repos
	}
	o.Runners = runnerPool(ctx, oc)
	if r, ok := o.Runners[strings.ToLower(*runnerName)]; ok {
		o.Runner = r
	} else if *runnerName != "opencode" {
		return fmt.Errorf("runner %q is not available (have: %s)", *runnerName, strings.Join(o.Runners.Names(), ", "))
	}
	if jm != "" {
		asker, aerr := pickAsker(*askerName, oc)
		if aerr != nil {
			return aerr
		}
		o.Judge = orchestrator.NewJudge(asker, jm)
	}

	v, err := o.Work(ctx, ticket, orchestrator.WorkConfig{
		Triage: orchestrator.TriagePolicy{
			MaxAttempts:   *maxAttempts,
			EscalateAfter: *escalateAfter,
			EscalateModel: *escalateModel,
		},
		MaxCostUSD: *maxCost,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	}
	if v.Status != orchestrator.StatusPassed {
		os.Exit(3)
	}
	return nil
}

func cmdDaemon(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository to work in")
	reposSpec := fs.String("repos", os.Getenv("ORCHESTRATOR_REPOS"), "repo:label routing, e.g. api-mobile=/p/a,api-server=/p/b")
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branches from")
	model := fs.String("model", os.Getenv("ORCHESTRATOR_MODEL"), "provider/model for the worker")
	escalateModel := fs.String("escalate-model", os.Getenv("ORCHESTRATOR_ESCALATE_MODEL"), "stronger model")
	judgeModel := fs.String("judge-model", os.Getenv("ORCHESTRATOR_JUDGE_MODEL"), "model for judgment criteria")
	queue := fs.String("queue", "Ready", "state to pull from")
	runnerName := fs.String("runner", "opencode", "default worker: opencode, codex or claude (a runner: label on the ticket wins)")
	watch := fs.Bool("watch", false, "subscribe to Raenil's event stream for instant pickup")
	poll := fs.Duration("poll", 0, "reconcile sweep interval (default 30s, or 5m with --watch)")
	concurrency := fs.Int("concurrency", 1, "tickets in flight")
	maxAttempts := fs.Int("max-attempts", 3, "hard stop before handing back")
	escalateAfter := fs.Int("escalate-after", 2, "attempt after which to escalate")
	maxCost := fs.Float64("max-cost", 0, "per-ticket cost ceiling")
	maxCostHour := fs.Float64("max-cost-hour", 0, "rolling hourly cost ceiling")
	stopFile := fs.String("stop-file", "", "kill switch path (default: <run-root>/STOP)")
	requireLabel := fs.String("require-label", "", "only take issues carrying this label, e.g. ready-for-agent")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rc, oc, err := clients()
	if err != nil {
		return err
	}
	// Only OpenCode needs its server; Codex and Claude are CLIs.
	if oc.BaseURL == "" && strings.EqualFold(*runnerName, "opencode") {
		return fmt.Errorf("OPENCODE_URL must be set to use opencode as the runner (or pick codex|claude)")
	}
	if *model == "" {
		return fmt.Errorf("a model is required: --model or ORCHESTRATOR_MODEL")
	}
	if *maxCostHour <= 0 {
		// Unattended plus paid API plus no ceiling is how people wake up to a
		// four-figure bill. Refuse rather than warn.
		return fmt.Errorf("--max-cost-hour is required for unattended operation")
	}

	jm := *judgeModel
	if jm == "" {
		jm = *escalateModel
	}
	o := &orchestrator.Orchestrator{
		Raenil: rc,
		Runner: oc,
		Cfg: orchestrator.Config{
			RunRoot: *runRoot, Repo: *repo, BaseRef: *baseRef,
			Model: *model, Timeout: *timeout,
		},
		Log: func(format string, a ...any) {
			fmt.Printf("%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, a...)...)
		},
	}
	if jm != "" {
		o.Judge = orchestrator.NewJudge(oc, jm)
	}
	if repos, rerr := parseRepos(*reposSpec); rerr != nil {
		return rerr
	} else {
		o.Repos = repos
	}
	o.Runners = runnerPool(ctx, oc)
	if r, ok := o.Runners[strings.ToLower(*runnerName)]; ok {
		o.Runner = r
	} else if *runnerName != "opencode" {
		return fmt.Errorf("runner %q is not available (have: %s)", *runnerName, strings.Join(o.Runners.Names(), ", "))
	}
	if len(o.Runners) > 1 {
		o.Log("runners available: %s — default %s (a runner: label on a ticket picks another)",
			strings.Join(o.Runners.Names(), ", "), o.Runner.Name())
	}

	// With a live event stream the sweep is only a backstop for dropped events,
	// so it can be slow. Without one it is the only way work is ever noticed.
	sweep := *poll
	if sweep == 0 {
		sweep = 30 * time.Second
		if *watch {
			sweep = 5 * time.Minute
		}
	}
	d := &orchestrator.Daemon{
		Orch: o,
		Cfg: orchestrator.DaemonConfig{
			ReadyState:    *queue,
			Watch:         *watch,
			PollInterval:  sweep,
			MaxConcurrent: *concurrency,
			MaxUSDPerHour: *maxCostHour,
			StopFile:      *stopFile,
			RequireLabel:  *requireLabel,
			Work: orchestrator.WorkConfig{
				Triage: orchestrator.TriagePolicy{
					MaxAttempts:   *maxAttempts,
					EscalateAfter: *escalateAfter,
					EscalateModel: *escalateModel,
				},
				MaxCostUSD: *maxCost,
			},
		},
	}
	return d.Run(ctx)
}

// firstSet returns the first environment variable of those named that is set.
func firstSet(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

func cmdPropose(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("propose", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository the ticket will be worked in")
	model := fs.String("model", firstSet("ORCHESTRATOR_PROPOSE_MODEL", "ORCHESTRATOR_JUDGE_MODEL", "ORCHESTRATOR_ESCALATE_MODEL"), "model to draft with")
	apply := fs.Bool("apply", false, "write the checklist to the ticket (default: print only)")
	skipVerify := fs.Bool("skip-verify", false, "do not check that the criteria fail today")
	askerName := fs.String("asker", "opencode", "who drafts: opencode, codex or claude")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("a ticket key is required, e.g. PP-42")
	}
	ticket := pos[0]

	rc, oc, cerr := clients()
	if cerr != nil {
		return cerr
	}
	// Only OpenCode needs its server; Codex and Claude are CLIs.
	if oc.BaseURL == "" && strings.EqualFold(*askerName, "opencode") {
		return fmt.Errorf("OPENCODE_URL must be set to use opencode as the asker (or pick codex|claude)")
	}
	if *model == "" {
		return fmt.Errorf("a model is required: --model or ORCHESTRATOR_ESCALATE_MODEL")
	}

	issue, err := rc.Issue(ctx, ticket)
	if err != nil {
		return err
	}
	facts, err := orchestrator.InspectRepo(*repo)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", *repo, err)
	}
	fmt.Printf("drafting criteria for %s with %s\n\n", issue.Key, *model)

	asker, aerr := pickAsker(*askerName, oc)
	if aerr != nil {
		return aerr
	}
	items, cost, err := orchestrator.Propose(ctx, asker, *model, issue, facts)
	if err != nil {
		return fmt.Errorf("%w (spent $%.4f)", err, cost)
	}
	fmt.Printf("drafted in %s for $%.4f\n\n", *model, cost)
	for i, it := range items {
		fmt.Printf("%d. [%s] %s\n", i+1, it.Kind, it.Text)
		if len(it.Check) > 0 {
			fmt.Printf("     %s\n", string(it.Check))
		}
	}

	if !*skipVerify {
		fmt.Println("\nchecking these actually fail on the repo as it stands…")
		gating, err := orchestrator.VerifyCriteriaFailToday(ctx, *repo, items)
		if err != nil {
			return err
		}
		for _, g := range gating {
			fmt.Println("  " + g)
		}
	}

	if !*apply {
		fmt.Println("\nnot written — re-run with --apply to put this on the ticket")
		return nil
	}
	if err := rc.ReplaceCriteria(ctx, issue.ID, items); err != nil {
		return err
	}
	fmt.Printf("\nwrote %d criteria to %s\n", len(items), issue.Key)
	return nil
}

func cmdBench(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	dir := fs.String("tasks", "bench", "directory of fixture tasks")
	modelsCSV := fs.String("models", os.Getenv("ORCHESTRATOR_MODEL"), "comma-separated provider/model list")
	runRoot := fs.String("run-root", "", "keep per-run evidence here")
	timeout := fs.Duration("timeout", 10*time.Minute, "bound each attempt")
	only := fs.String("only", "", "run just this task")
	asJSON := fs.Bool("json", false, "print results as JSON")
	dryRun := fs.Bool("dry-run", false, "evaluate the fixtures with no agent, to check the criteria themselves")
	hideGuards := fs.Bool("hide-guards", false, "hide the policy criteria and anti-cheating rule from the worker (research only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tasks, err := orchestrator.LoadBenchTasks(*dir)
	if err != nil {
		return fmt.Errorf("load tasks from %s: %w", *dir, err)
	}
	if *only != "" {
		var kept []orchestrator.BenchTask
		for _, t := range tasks {
			if t.Name == *only {
				kept = append(kept, t)
			}
		}
		tasks = kept
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no tasks found in %s", *dir)
	}

	var runner orchestrator.Runner = noopRunner{}
	modelList := []string{"none"}
	if !*dryRun {
		_, oc, err := clientsOptional()
		if err != nil {
			return err
		}
		if oc.BaseURL == "" {
			return fmt.Errorf("OPENCODE_URL must be set (or pass --dry-run)")
		}
		runner = oc
		modelList = strings.Split(*modelsCSV, ",")
		for i := range modelList {
			modelList[i] = strings.TrimSpace(modelList[i])
		}
		if len(modelList) == 0 || modelList[0] == "" {
			return fmt.Errorf("--models is required, e.g. --models opencode-go/glm-5.3-flash,opencode-go/deepseek-v4.1-flash")
		}
	}

	results := orchestrator.RunBench(ctx, tasks, modelList, runner, orchestrator.BenchOptions{
		Timeout:    *timeout,
		RunRoot:    *runRoot,
		HideGuards: *hideGuards,
		Log:        func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	})

	fmt.Println()
	if *asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Print(orchestrator.BenchReport(results))
	return nil
}

// clientsOptional builds only the agent client; bench needs no tracker.
func clientsOptional() (*orchestrator.RaenilClient, *orchestrator.OpenCodeRunner, error) {
	return nil, &orchestrator.OpenCodeRunner{
		BaseURL: strings.TrimRight(os.Getenv("OPENCODE_URL"), "/"),
	}, nil
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	if err := fs.Parse(args); err != nil {
		return err
	}
	stop := filepath.Join(*runRoot, "STOP")
	if _, err := os.Stat(stop); err == nil {
		fmt.Printf("KILL SWITCH PRESENT: %s — the daemon will not start work\n\n", stop)
	}
	m := &orchestrator.LeaseManager{Dir: filepath.Join(*runRoot, "leases")}
	held, err := m.Held()
	if err != nil {
		return err
	}
	if len(held) == 0 {
		fmt.Println("no tickets held")
		return nil
	}
	fmt.Printf("%-12s %-8s %-18s %s\n", "TICKET", "PID", "HOST", "HEARTBEAT")
	for _, l := range held {
		age := time.Since(l.Heartbeat).Round(time.Second)
		fmt.Printf("%-12s %-8d %-18s %s ago\n", l.Ticket, l.PID, l.Host, age)
	}
	return nil
}

// cmdReview backs `verify` and `finish`: the reviewer's half of the loop, where
// the code may have been written by anyone and the checks decide regardless.
func cmdReview(ctx context.Context, args []string, finish bool) error {
	name := "verify"
	if finish {
		name = "finish"
	}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	repo := fs.String("repo", ".", "repository the ticket is worked in")
	reposSpec := fs.String("repos", os.Getenv("ORCHESTRATOR_REPOS"), "repo:label routing, e.g. api-mobile=/p/a,api-server=/p/b")
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branched from")
	attempt := fs.Int("attempt", 90, "run directory to write evidence into")
	judgeModel := fs.String("judge-model", os.Getenv("ORCHESTRATOR_JUDGE_MODEL"), "model for judgment criteria")
	askerName := fs.String("asker", "opencode", "who answers judgment criteria: opencode, codex or claude")
	asJSON := fs.Bool("json", false, "print the verdict as JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("a ticket key is required, e.g. PP-42")
	}

	rc, oc, err := clients()
	if err != nil {
		return err
	}
	o := &orchestrator.Orchestrator{
		Raenil: rc,
		Runner: noopRunner{},
		Cfg: orchestrator.Config{
			RunRoot: *runRoot, Repo: *repo, BaseRef: *baseRef,
		},
		Log: func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	}
	repos, rerr := parseRepos(*reposSpec)
	if rerr != nil {
		return rerr
	}
	o.Repos = repos
	if *judgeModel != "" {
		asker, aerr := pickAsker(*askerName, oc)
		if aerr != nil {
			return aerr
		}
		o.Judge = orchestrator.NewJudge(asker, *judgeModel)
	}

	var v orchestrator.Verdict
	if finish {
		v, err = o.Finish(ctx, pos[0], *attempt)
	} else {
		v, _, err = o.Verify(ctx, pos[0], *attempt)
	}
	if err != nil {
		return err
	}
	if *asJSON {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	}
	if v.Status != orchestrator.StatusPassed {
		os.Exit(3)
	}
	return nil
}

// noopRunner backs `orchestrator check`: evaluate the criteria against the repo
// exactly as it stands, with no agent and no cost.
type noopRunner struct{}

func (noopRunner) Name() string { return "none" }
func (noopRunner) Run(context.Context, orchestrator.RunRequest) (orchestrator.RunResult, error) {
	return orchestrator.RunResult{}, nil
}

// cmdHost runs this machine as a runner host: it reports which harnesses are
// ready here and runs the jobs queued from Raenil's web UI. It serves every
// workspace the token reaches, or only RAENIL_WORKSPACE when set.
func cmdHost(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	hostname, _ := os.Hostname()
	name := fs.String("name", strings.TrimSuffix(hostname, ".local"), "how this machine appears in Raenil")
	poll := fs.Duration("poll", 3*time.Second, "how often to ask for queued work")
	home, _ := os.UserHomeDir()
	repo := fs.String("repo", ".", "repository a ticket is worked in when it names none")
	reposSpec := fs.String("repos", os.Getenv("ORCHESTRATOR_REPOS"), "repo:label routing, e.g. api-mobile=/p/a,api-server=/p/b")
	runRoot := fs.String("run-root", filepath.Join(home, ".raenil", "runs"), "where run directories and worktrees live")
	baseRef := fs.String("base", "HEAD", "what a worktree branches from")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	maxAttempts := fs.Int("max-attempts", 3, "attempts before a ticket is handed back")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rc, oc, err := clients()
	if err != nil {
		return err
	}

	var serve []*orchestrator.RaenilClient
	if rc.Workspace != "" {
		serve = append(serve, rc)
	} else {
		spaces, err := rc.Workspaces(ctx)
		if err != nil {
			return fmt.Errorf("list workspaces: %w", err)
		}
		for _, w := range spaces {
			serve = append(serve, &orchestrator.RaenilClient{BaseURL: rc.BaseURL, Token: rc.Token, Workspace: w.Slug})
		}
	}

	// Claude and Codex are always offered, so the dashboard can say what is
	// missing; OpenCode only when a server is configured.
	runners := orchestrator.RunnerSet{"claude": claudeRunner(), "codex": &orchestrator.CodexRunner{}}
	if oc.BaseURL != "" {
		runners["opencode"] = oc
	}

	repos, err := parseRepos(*reposSpec)
	if err != nil {
		return err
	}
	logf := func(f string, a ...any) { fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05 ")+f+"\n", a...) }

	// Running a ticket from the dashboard is `orchestrator work KEY --handoff`
	// with the agent's harness, model and allowlist: attempts, retries, and a
	// kept worktree for review. The agent's harness decides — a runner: label
	// on the ticket does not override the agent that was asked.
	runTicket := func(ctx context.Context, c *orchestrator.RaenilClient, job orchestrator.ClaimedJob) (any, error) {
		a := job.Agent
		if a == nil {
			return nil, errors.New("the job names no agent")
		}
		if a.Status == "paused" {
			return nil, fmt.Errorf("%s is paused", a.Name)
		}
		if job.IssueKey == "" {
			return nil, errors.New("the job names no ticket")
		}
		runner, err := agentRunner(*a, oc)
		if err != nil {
			return nil, err
		}
		model := orchestrator.AgentModel(*a)
		if model == "" {
			return nil, fmt.Errorf("%s has no model: set one on its Harness page", a.Name)
		}
		o := &orchestrator.Orchestrator{
			Raenil:   c,
			Runner:   runner,
			AgentID:  a.ID,
			HostName: *name,
			Repos:    repos,
			Cfg: orchestrator.Config{
				RunRoot: *runRoot, Repo: *repo, BaseRef: *baseRef,
				Model: model, Timeout: *timeout, Handoff: true,
			},
			Leases: &orchestrator.LeaseManager{Dir: filepath.Join(*runRoot, "leases")},
			Log:    logf,
		}
		if asker, ok := runner.(orchestrator.Asker); ok {
			o.Judge = orchestrator.NewJudge(asker, model)
		}
		return o.Work(ctx, job.IssueKey, orchestrator.WorkConfig{
			Triage: orchestrator.TriagePolicy{MaxAttempts: *maxAttempts, EscalateAfter: *maxAttempts},
		})
	}

	h := &orchestrator.Host{
		Name:      *name,
		Version:   "orchestrator",
		Clients:   serve,
		Runners:   runners,
		RunTicket: runTicket,
		RunnerFor: func(a models.Agent) (orchestrator.Runner, error) { return agentRunner(a, oc) },
		Repos:     repos,
		Repo:      *repo,
		MCPURL:    rc.BaseURL + "/mcp",
		Poll:      *poll,
		Logf:      logf,
	}
	names := make([]string, len(serve))
	for i, c := range serve {
		names[i] = c.Workspace
	}
	fmt.Fprintf(os.Stderr, "host %s serving %s\n", *name, strings.Join(names, ", "))
	err = h.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// agentRunner builds the runner an agent asked for, with its own settings.
func agentRunner(a models.Agent, oc *orchestrator.OpenCodeRunner) (orchestrator.Runner, error) {
	switch a.Harness {
	case "claude":
		r := claudeRunner()
		r.AllowedTools = append(r.AllowedTools, a.AllowedTools...)
		r.Effort, r.MaxTurns = a.Effort, a.MaxTurns
		return r, nil
	case "codex":
		return &orchestrator.CodexRunner{}, nil
	case "opencode":
		if oc == nil || oc.BaseURL == "" {
			return nil, errors.New("OpenCode is not set up on this host: set OPENCODE_URL")
		}
		return oc, nil
	}
	return nil, fmt.Errorf("unknown harness %q", a.Harness)
}
