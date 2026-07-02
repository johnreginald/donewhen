// Command kanri is the single-binary server + tooling for the Kanri tracker.
//
// Subcommands:
//
//	kanri serve                 run the HTTP API + SSE + Web Push + MCP endpoint
//	kanri migrate               apply DB migrations and exit
//	kanri mcp                   run the MCP server over stdio (local fallback)
//	kanri token <name>          create an API token for the first user
//	kanri user <email> <pass>   create the initial user
//	kanri genvapid              print a fresh VAPID keypair
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kanri/internal/api"
	"kanri/internal/auth"
	"kanri/internal/config"
	"kanri/internal/db"
	"kanri/internal/events"
	appmcp "kanri/internal/mcp"
	"kanri/internal/push"
	"kanri/internal/service"
	"kanri/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("kanri: ")

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "migrate":
		runMigrate()
	case "mcp":
		runMCPStdio()
	case "token":
		runToken(os.Args[2:])
	case "user":
		runUser(os.Args[2:])
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
	fmt.Print(`kanri — self-hosted issue tracker

usage:
  kanri serve                 run the server (default)
  kanri migrate               apply DB migrations and exit
  kanri mcp                   run the MCP server over stdio
  kanri token <name>          create an API token for the first user
  kanri user <email> <pass>   create the initial user
  kanri genvapid              print a fresh VAPID keypair
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
	cfg := mustConfig()
	ctx := context.Background()
	st := connect(ctx, cfg)
	defer st.Pool().Close()

	u, err := st.FirstUser(ctx)
	if err != nil {
		log.Fatalf("no user yet — run `kanri user <email> <pass>` first")
	}
	raw, err := auth.RandomToken(32)
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	secret := "kanri_" + raw
	if _, err := st.CreateAPIToken(ctx, u.ID, name, auth.HashToken(secret)); err != nil {
		log.Fatalf("token: %v", err)
	}
	fmt.Printf("API token (%s) for %s — store it now, shown once:\n\n  %s\n\n", name, u.Email, secret)
}

func runUser(args []string) {
	if len(args) < 2 {
		log.Fatalf("usage: kanri user <email> <password>")
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
	fmt.Printf("KANRI_VAPID_PRIVATE=%s\nKANRI_VAPID_PUBLIC=%s\n", priv, pub)
}
