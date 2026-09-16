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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	case "status":
		err = cmdStatus(args)
	case "bench":
		err = cmdBench(ctx, args)
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
  orchestrator daemon                 work the Ready queue unattended
  orchestrator status                 show held leases and the kill switch
  orchestrator bench                  compare models over the fixture tasks

Flags (daemon only):
  --queue STATE      state to pull from (default: Ready)
  --poll DUR         how often to check the queue (default: 30s)
  --concurrency N    tickets in flight (default: 1 — see docs/ORCHESTRATOR.md)
  --max-cost-hour U  halt once spend in a rolling hour exceeds this
  --stop-file PATH   kill switch; the daemon halts while it exists

Flags (work only):
  --max-attempts N   hard stop before handing back to a human (default: 3)
  --escalate-after N attempt after which to use the stronger model (default: 2)
  --escalate-model M the stronger model (default: $ORCHESTRATOR_ESCALATE_MODEL)
  --max-cost USD     stop once a ticket has cost this much across attempts
  --judge-model M    model for judgment criteria (default: the escalate model)

Flags (run/check/work):
  --repo PATH        repository to work in (default: current directory)
  --run-root PATH    where run dirs live (default: .orchestrator)
  --base REF         what the worktree branches from (default: HEAD)
  --model NAME       provider/model for the worker (default: $ORCHESTRATOR_MODEL)
  --attempt N        attempt number (default: 1)
  --timeout DUR      bound one attempt (default: 30m)
  --json             print the verdict as JSON

Environment:
  RAENIL_URL         tracker base URL, e.g. https://tracker.example.com
  RAENIL_TOKEN       API token (raenil token <name>)
  RAENIL_WORKSPACE   workspace slug or id, when the token spans several
  OPENCODE_URL       running 'opencode serve', e.g. http://127.0.0.1:4096
  ORCHESTRATOR_MODEL default worker model, e.g. opencode-go/glm-5.3-flash
  ORCHESTRATOR_ESCALATE_MODEL  stronger model for escalation and judgment
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
	if !ok {
		return fmt.Errorf("one or more dependencies are unreachable")
	}
	return nil
}

func cmdRun(ctx context.Context, args []string, checkOnly bool) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository to work in")
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branches from")
	model := fs.String("model", os.Getenv("ORCHESTRATOR_MODEL"), "provider/model for the worker")
	attempt := fs.Int("attempt", 1, "attempt number")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	asJSON := fs.Bool("json", false, "print the verdict as JSON")
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
		if oc.BaseURL == "" {
			return fmt.Errorf("OPENCODE_URL must be set to run an agent (use 'check' to evaluate without one)")
		}
		if *model == "" {
			return fmt.Errorf("a model is required: --model or ORCHESTRATOR_MODEL " +
				"(needs a provider prefix, e.g. opencode-go/glm-5.3-flash)")
		}
	}

	var runner orchestrator.Runner = oc
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
		},
		Log: func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
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
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branches from")
	model := fs.String("model", os.Getenv("ORCHESTRATOR_MODEL"), "provider/model for the worker")
	escalateModel := fs.String("escalate-model", os.Getenv("ORCHESTRATOR_ESCALATE_MODEL"), "stronger model")
	judgeModel := fs.String("judge-model", "", "model for judgment criteria")
	maxAttempts := fs.Int("max-attempts", 3, "hard stop before handing back")
	escalateAfter := fs.Int("escalate-after", 2, "attempt after which to escalate")
	maxCost := fs.Float64("max-cost", 0, "stop once a ticket has cost this much")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	asJSON := fs.Bool("json", false, "print the verdict as JSON")
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
	if oc.BaseURL == "" {
		return fmt.Errorf("OPENCODE_URL must be set")
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
			Model: *model, Timeout: *timeout,
		},
		Log: func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	}
	if jm != "" {
		o.Judge = orchestrator.NewJudge(oc, jm)
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
	runRoot := fs.String("run-root", ".orchestrator", "where run directories live")
	baseRef := fs.String("base", "HEAD", "what the worktree branches from")
	model := fs.String("model", os.Getenv("ORCHESTRATOR_MODEL"), "provider/model for the worker")
	escalateModel := fs.String("escalate-model", os.Getenv("ORCHESTRATOR_ESCALATE_MODEL"), "stronger model")
	judgeModel := fs.String("judge-model", "", "model for judgment criteria")
	queue := fs.String("queue", "Ready", "state to pull from")
	poll := fs.Duration("poll", 30*time.Second, "how often to check the queue")
	concurrency := fs.Int("concurrency", 1, "tickets in flight")
	maxAttempts := fs.Int("max-attempts", 3, "hard stop before handing back")
	escalateAfter := fs.Int("escalate-after", 2, "attempt after which to escalate")
	maxCost := fs.Float64("max-cost", 0, "per-ticket cost ceiling")
	maxCostHour := fs.Float64("max-cost-hour", 0, "rolling hourly cost ceiling")
	stopFile := fs.String("stop-file", "", "kill switch path (default: <run-root>/STOP)")
	timeout := fs.Duration("timeout", 30*time.Minute, "bound one attempt")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rc, oc, err := clients()
	if err != nil {
		return err
	}
	if oc.BaseURL == "" {
		return fmt.Errorf("OPENCODE_URL must be set")
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

	d := &orchestrator.Daemon{
		Orch: o,
		Cfg: orchestrator.DaemonConfig{
			ReadyState:    *queue,
			PollInterval:  *poll,
			MaxConcurrent: *concurrency,
			MaxUSDPerHour: *maxCostHour,
			StopFile:      *stopFile,
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

func cmdBench(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	dir := fs.String("tasks", "bench", "directory of fixture tasks")
	modelsCSV := fs.String("models", os.Getenv("ORCHESTRATOR_MODEL"), "comma-separated provider/model list")
	runRoot := fs.String("run-root", "", "keep per-run evidence here")
	timeout := fs.Duration("timeout", 10*time.Minute, "bound each attempt")
	only := fs.String("only", "", "run just this task")
	asJSON := fs.Bool("json", false, "print results as JSON")
	dryRun := fs.Bool("dry-run", false, "evaluate the fixtures with no agent, to check the criteria themselves")
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
		Timeout: *timeout,
		RunRoot: *runRoot,
		Log:     func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
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

// noopRunner backs `orchestrator check`: evaluate the criteria against the repo
// exactly as it stands, with no agent and no cost.
type noopRunner struct{}

func (noopRunner) Name() string { return "none" }
func (noopRunner) Run(context.Context, orchestrator.RunRequest) (orchestrator.RunResult, error) {
	return orchestrator.RunResult{}, nil
}
