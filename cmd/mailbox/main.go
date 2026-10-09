package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/controlprobe"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/httpserver"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpserver"
	"github.com/petarnenov/bot-space/internal/ratelimit"
	management "github.com/petarnenov/bot-space/internal/web"
	"github.com/petarnenov/bot-space/internal/workspaces"
	"github.com/petarnenov/bot-space/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("command_failed", "reason", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	command := "serve"
	if len(os.Args) >= 2 {
		command = os.Args[1]
	}
	if (len(os.Args) > 2 && command != "bootstrap-owner") || (command != "serve" && command != "migrate" && command != "bootstrap-owner") {
		return fmt.Errorf("usage: mailbox [serve|migrate|bootstrap-owner]")
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	versions, err := migrations.Bundled()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if command == "migrate" {
		if err := database.Migrate(ctx, cfg.DatabaseURL, versions); err != nil {
			return err
		}
		logger.Info("migrations_complete")
		return nil
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.MaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	if command == "bootstrap-owner" {
		flags := flag.NewFlagSet("bootstrap-owner", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		githubID := flags.Int64("github-user-id", 0, "Immutable GitHub user ID")
		slug := flags.String("workspace", "", "Workspace slug")
		if flags.Parse(os.Args[2:]) != nil || flags.NArg() != 0 {
			return fmt.Errorf("invalid bootstrap arguments")
		}
		workspace, err := (&workspaces.Store{Pool: pool}).Bootstrap(ctx, *githubID, *slug)
		if err != nil {
			return err
		}
		logger.Info("workspace_bootstrapped", "workspace_id", workspace.ID)
		return nil
	}
	identityConfig, err := config.LoadIdentity(os.Getenv)
	if err != nil {
		return err
	}
	mailboxConfig, err := config.LoadMailbox(os.Getenv, identityConfig.BaseURL)
	if err != nil {
		return err
	}
	server := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, versions) }, logger)
	loginLimit, mcpPeerLimit := ratelimit.New(20, 5, 10000, nil), ratelimit.New(120, 60, 10000, nil)
	server.Use(func(next http.Handler) http.Handler { return ratelimit.PeerAdmission(loginLimit, mcpPeerLimit, next) })
	if identityConfig.Enabled {
		web := &identity.Web{Config: identityConfig, Sessions: &identity.Sessions{Pool: pool}, Workspaces: &workspaces.Store{Pool: pool}, Provider: identity.GitHubProvider()}
		web.Register(server)
		(&agents.Web{Store: &agents.Store{Pool: pool}}).Register(server, web)
		(&management.Management{Browser: web, Teams: &workspaces.Store{Pool: pool}, Agents: &agents.Store{Pool: pool}, MailboxEnabled: mailboxConfig.Enabled}).Register(server)
	}
	if mailboxConfig.Enabled {
		store, err := mailbox.New(pool, mailboxConfig.CursorKey)
		if err != nil {
			return fmt.Errorf("mailbox configuration is invalid")
		}
		server.Handle("/mcp", mcpserver.New(store, mailboxConfig.AllowedOrigins))
	}
	if token := os.Getenv("CONTROL_PROBE_TOKEN"); token != "" {
		handler, shutdown, err := controlprobe.Wrap(server.HTTP.Handler, token)
		if err != nil {
			return err
		}
		defer shutdown()
		server.HTTP.Handler = handler
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return fmt.Errorf("HTTP listener could not be opened")
	}
	logger.Info("server_started", "port", cfg.Port)
	return server.Serve(ctx, listener)
}
