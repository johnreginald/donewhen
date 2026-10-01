// Command donewhen is the single-binary server + tooling for the DoneWhen tracker.
//
// Subcommands:
//
//	donewhen serve                 run the HTTP API + SSE + Web Push + MCP endpoint
//	donewhen migrate               apply DB migrations and exit
//	donewhen mcp                   run the MCP server over stdio (local fallback)
//	donewhen token <name> [ws]     create an API token, optionally pinned to a workspace
//	donewhen user <email> <pass>   create the initial user
//	donewhen workspace ...         list/create workspaces and grant access
//	donewhen demo [--workspace s]  seed a sample "Demo" workspace
//	donewhen genvapid              print a fresh VAPID keypair
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/johnreginald/donewhen/internal/api"
	"github.com/johnreginald/donewhen/internal/auth"
	"github.com/johnreginald/donewhen/internal/config"
	"github.com/johnreginald/donewhen/internal/db"
	"github.com/johnreginald/donewhen/internal/demo"
	"github.com/johnreginald/donewhen/internal/events"
	appmcp "github.com/johnreginald/donewhen/internal/mcp"
	"github.com/johnreginald/donewhen/internal/push"
	"github.com/johnreginald/donewhen/internal/service"
	"github.com/johnreginald/donewhen/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("donewhen: ")

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "migrate":
		runMigrate()
	case "migrate-pending":
		runMigratePending()
	case "mcp":
		runMCPStdio()
	case "token":
		runToken(os.Args[2:])
	case "user":
		runUser(os.Args[2:])
	case "workspace", "ws":
		runWorkspace(os.Args[2:])
	case "demo":
		runDemo(os.Args[2:])
	case "genvapid":
		runGenVAPID()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`donewhen — self-hosted issue tracker

usage:
  donewhen serve                 run the server (default)
  donewhen migrate               apply DB migrations and exit
  donewhen migrate-pending       print pending destructive migrations (comma-separated)
  donewhen mcp                   run the MCP server over stdio
  donewhen token <name> [ws]     create an API token; pass a workspace slug to pin it
  donewhen user <email> <pass>   create the initial user
  donewhen genvapid              print a fresh VAPID keypair

  donewhen demo [--workspace <slug>]           seed a sample workspace (needs a user; safe to repeat)

  donewhen workspace list                      show workspaces + member counts
  donewhen workspace create <name> <prefix>    create a workspace
  donewhen workspace add <slug> <email> [role] grant a user access (owner|admin|member)
`)
}

func mustConfig() config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}

func connect(ctx context.Context, cfg config.Config) *store.Store {
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	return store.New(pool, cfg.IssuePrefix)
}

func runServe() {
	cfg := mustConfig()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st := connect(ctx, cfg)
	bus := events.NewBus()
	svc := service.New(st, bus)

	if config.Getenv("DONEWHEN_DEMO") == "1" {
		seedDemoOnStart(ctx, st, svc)
	}

	// Web Push consumer.
	notifier := push.NewNotifier(st, bus, cfg)
	go notifier.Run(ctx)

	// MCP endpoint on the same server.
	mcpHandler := appmcp.NewHandler(svc, st, cfg)

	srv := api.NewServer(cfg, st, svc, bus, mcpHandler)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (base %s, push=%v)", cfg.ListenAddr, cfg.BaseURL, cfg.PushEnabled())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutCtx)
	st.Pool().Close()
}

func runMigrate() {
	cfg := mustConfig()
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations up to date")
}

// runMigratePending prints the pending destructive migrations, comma-separated,
// for `make migrate` to back up before applying. Prints nothing when none.
func runMigratePending() {
	cfg := mustConfig()
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	names, err := db.PendingDestructive(ctx, pool)
	if err != nil {
		log.Fatalf("migrate-pending: %v", err)
	}
	fmt.Println(strings.Join(names, ","))
}

func runMCPStdio() {
	cfg := mustConfig()
	ctx := context.Background()
	st := connect(ctx, cfg)
	defer st.Pool().Close()
	svc := service.New(st, events.NewBus())
	if err := appmcp.ServeStdio(svc, st, cfg); err != nil {
		log.Fatalf("mcp stdio: %v", err)
	}
}

func runToken(args []string) {
	name := "token"
	if len(args) > 0 {
		name = args[0]
	}
	// A pinned token can only ever act on one workspace — the recipe for giving
	// an agent working in one repo access to just that repo's tracker.
	wsRef := ""
	if len(args) > 1 {
		wsRef = args[1]
	}
	cfg := mustConfig()
	ctx := context.Background()
	st := connect(ctx, cfg)
	defer st.Pool().Close()

	u, err := st.FirstUser(ctx)
	if err != nil {
		log.Fatalf("no user yet — run `donewhen user <email> <pass>` first")
	}
	raw, err := auth.RandomToken(32)
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	secret := auth.TokenPrefix + raw

	var pin *string
	scope := "all your workspaces"
	if wsRef != "" {
		ws, err := st.ResolveWorkspace(ctx, wsRef)
		if err != nil {
			log.Fatalf("workspace %q not found", wsRef)
		}
		pin = &ws.ID
		scope = "workspace " + ws.Name
	}
	if _, err := st.CreateAPIToken(ctx, u.ID, name, auth.HashToken(secret), pin); err != nil {
		log.Fatalf("token: %v", err)
	}
	fmt.Printf("API token (%s) for %s, scoped to %s — store it now, shown once:\n\n  %s\n\n",
		name, u.Email, scope, secret)
}

func runWorkspace(args []string) {
	if len(args) == 0 {
		log.Fatalf("usage: donewhen workspace <list|create|add> ...")
	}
	cfg := mustConfig()
	ctx := context.Background()
	st := connect(ctx, cfg)
	defer st.Pool().Close()

	switch args[0] {
	case "list":
		items, err := st.ListWorkspaces(ctx)
		if err != nil {
			log.Fatalf("list workspaces: %v", err)
		}
		if len(items) == 0 {
			fmt.Println("no workspaces yet — create one with `donewhen workspace create <name> <prefix>`")
			return
		}
		fmt.Printf("%-4s  %-24s  %-24s  %s\n", "KEY", "SLUG", "NAME", "MEMBERS")
		for _, w := range items {
			members, _ := st.ListMembers(ctx, w.ID)
			fmt.Printf("%-4s  %-24s  %-24s  %d\n", w.KeyPrefix, w.Slug, w.Name, len(members))
		}

	case "create":
		if len(args) < 3 {
			log.Fatalf("usage: donewhen workspace create <name> <prefix>")
		}
		name, prefix := args[1], args[2]
		if err := store.ValidatePrefix(prefix, st.ReservedPrefix()); err != nil {
			log.Fatalf("%v", err)
		}
		// The first user owns anything created from the command line.
		owner := ""
		if u, err := st.FirstUser(ctx); err == nil {
			owner = u.ID
		}
		w, err := st.CreateWorkspace(ctx, name, "", prefix, owner)
		if err != nil {
			log.Fatalf("create workspace: %v", err)
		}
		fmt.Printf("created workspace %s (%s), issue keys will be %s-1, %s-2, …\n",
			w.Name, w.Slug, w.KeyPrefix, w.KeyPrefix)

	case "add":
		if len(args) < 3 {
			log.Fatalf("usage: donewhen workspace add <slug> <email> [owner|admin|member]")
		}
		role := "member"
		if len(args) > 3 {
			role = args[3]
		}
		w, err := st.ResolveWorkspace(ctx, args[1])
		if err != nil {
			log.Fatalf("workspace %q not found", args[1])
		}
		u, _, err := st.GetUserByEmail(ctx, args[2])
		if err != nil {
			log.Fatalf("no account for %s — create it with `donewhen user` first", args[2])
		}
		if err := st.AddMember(ctx, w.ID, u.ID, role); err != nil {
			log.Fatalf("add member: %v", err)
		}
		fmt.Printf("%s is now %s of %s\n", u.Email, role, w.Name)

	default:
		log.Fatalf("unknown workspace command %q (list|create|add)", args[0])
	}
}

func runUser(args []string) {
	if len(args) < 2 {
		log.Fatalf("usage: donewhen user <email> <password>")
	}
	email, pass := args[0], args[1]
	if len(pass) < 8 {
		log.Fatalf("password must be at least 8 characters")
	}
	cfg := mustConfig()
	ctx := context.Background()
	st := connect(ctx, cfg)
	defer st.Pool().Close()

	hash, err := auth.HashPassword(pass)
	if err != nil {
		log.Fatalf("hash: %v", err)
	}
	u, err := st.CreateUser(ctx, email, hash)
	if err != nil {
		log.Fatalf("create user: %v", err)
	}
	fmt.Printf("created user %s (%s)\n", u.Email, u.ID)
}

func runGenVAPID() {
	priv, pub, err := push.GenerateVAPIDKeys()
	if err != nil {
		log.Fatalf("genvapid: %v", err)
	}
	fmt.Printf("DONEWHEN_VAPID_PRIVATE=%s\nDONEWHEN_VAPID_PUBLIC=%s\n", priv, pub)
}

// seedDemoOnStart honours DONEWHEN_DEMO=1: seed the demo if a user exists and
// the demo is not there yet. Never fatal, since the demo is optional.
func seedDemoOnStart(ctx context.Context, st *store.Store, svc *service.Service) {
	u, err := st.FirstUser(ctx)
	if err != nil {
		log.Printf("DONEWHEN_DEMO=1: no user yet; create one, then restart or run `donewhen demo`")
		return
	}
	res, err := demo.Seed(ctx, svc, u.ID, "")
	switch {
	case errors.Is(err, demo.ErrExists):
		log.Printf("demo already exists")
	case err != nil:
		log.Printf("demo: %v", err)
	default:
		log.Printf("demo workspace %q seeded: %d issues", res.WorkspaceSlug, res.Issues)
	}
}

func runDemo(args []string) {
	slug := demo.Slug
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--workspace" && i+1 < len(args):
			slug = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--workspace="):
			slug = strings.TrimPrefix(args[i], "--workspace=")
		default:
			log.Fatalf("usage: donewhen demo [--workspace <slug>]")
		}
	}
	cfg := mustConfig()
	ctx := context.Background()
	st := connect(ctx, cfg)
	defer st.Pool().Close()

	u, err := st.FirstUser(ctx)
	if err != nil {
		log.Fatalf("no user yet — run `donewhen user <email> <pass>` first")
	}
	res, err := demo.Seed(ctx, service.New(st, events.NewBus()), u.ID, slug)
	if errors.Is(err, demo.ErrExists) {
		fmt.Printf("demo already exists (workspace %q), nothing changed\n", slug)
		return
	}
	if err != nil {
		log.Fatalf("demo: %v", err)
	}
	fmt.Printf("seeded workspace %q: %d epics, %d issues (%d in review), %d documents.\nSign in as %s and open it.\n",
		res.WorkspaceSlug, res.Epics, res.Issues, res.InReview, res.Documents, u.Email)
}
