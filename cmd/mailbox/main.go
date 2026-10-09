package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"github.com/petarnenov/bot-space/internal/githubapp"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/petarnenov/bot-space/internal/agents"
	"github.com/petarnenov/bot-space/internal/backlog"
	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/control"
	"github.com/petarnenov/bot-space/internal/controlevents"
	"github.com/petarnenov/bot-space/internal/controlprobe"
	"github.com/petarnenov/bot-space/internal/councilstore"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/httpserver"
	"github.com/petarnenov/bot-space/internal/identity"
	"github.com/petarnenov/bot-space/internal/mailbox"
	"github.com/petarnenov/bot-space/internal/mcpserver"
	"github.com/petarnenov/bot-space/internal/orchestration"
	"github.com/petarnenov/bot-space/internal/ratelimit"
	"github.com/petarnenov/bot-space/internal/repositoryaccess"
	"github.com/petarnenov/bot-space/internal/runneridentity"
	"github.com/petarnenov/bot-space/internal/tasks"
	"github.com/petarnenov/bot-space/internal/taskweb"
	management "github.com/petarnenov/bot-space/internal/web"
	"github.com/petarnenov/bot-space/internal/workspaces"
	"github.com/petarnenov/bot-space/migrations"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	if (len(os.Args) > 2 && command != "bootstrap-owner" && command != "bootstrap-project") || (command != "serve" && command != "migrate" && command != "bootstrap-owner" && command != "bootstrap-project") {
		return fmt.Errorf("usage: mailbox [serve|migrate|bootstrap-owner|bootstrap-project]")
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
	if command == "bootstrap-project" {
		flags := flag.NewFlagSet("bootstrap-project", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		workspace := flags.String("workspace", "", "Workspace slug")
		repository := flags.String("repository", "", "GitHub owner/repository")
		if flags.Parse(os.Args[2:]) != nil || flags.NArg() != 0 {
			return errors.New("invalid project bootstrap arguments")
		}
		parts := strings.Split(*repository, "/")
		if len(parts) != 2 {
			return errors.New("repository must be owner/name")
		}
		installation, e := strconv.ParseInt(os.Getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
		if e != nil {
			return errors.New("GitHub App installation configuration required")
		}
		app, e := githubapp.New(os.Getenv("GITHUB_APP_CLIENT_ID"), installation, []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")))
		if e != nil {
			return e
		}
		checker, e := repositoryaccess.New(app.Token)
		if e != nil {
			return e
		}
		repo, e := checker.Resolve(ctx, parts[0], parts[1])
		if e != nil {
			return e
		}
		project, e := (&runneridentity.Store{Pool: pool}).ConfigureProject(ctx, *workspace, repo)
		if e != nil {
			return e
		}
		logger.Info("project_bootstrapped", "project_id", project, "repository_id", repo.ID)
		return nil
	}
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
	runnerIdentityConfig, err := config.LoadRunnerIdentity(os.Getenv)
	if err != nil {
		return err
	}
	if runnerIdentityConfig.Enabled && !identityConfig.Enabled {
		return errors.New("runner identity requires GitHub browser authentication")
	}
	server := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, versions) }, logger)
	loginLimit, mcpPeerLimit := ratelimit.New(20, 5, 10000, nil), ratelimit.New(120, 60, 10000, nil)
	server.Use(func(next http.Handler) http.Handler { return ratelimit.PeerAdmission(loginLimit, mcpPeerLimit, next) })
	var runnerIdentities *runneridentity.Store
	if identityConfig.Enabled {
		web := &identity.Web{Config: identityConfig, Sessions: &identity.Sessions{Pool: pool}, Workspaces: &workspaces.Store{Pool: pool}, Provider: identity.GitHubProvider()}
		web.Register(server)
		if runnerIdentityConfig.Enabled {
			authority, err := repositoryaccess.New(runnerIdentityConfig.AppTokens.Token)
			if err != nil {
				return err
			}
			runnerIdentities = &runneridentity.Store{Pool: pool, Authority: authority}
			(&runneridentity.Web{Store: runnerIdentities, Browser: web, ControlEndpoint: runnerIdentityConfig.ControlEndpoint, ControlCA: runnerIdentityConfig.ControlCA}).Register(server)
			(&backlog.Web{Store: &backlog.Store{Pool: pool, Sessions: web.Sessions, Authority: authority}, Browser: web}).Register(server)
		}
		(&agents.Web{Store: &agents.Store{Pool: pool}}).Register(server, web)
		(&management.Management{Browser: web, Teams: &workspaces.Store{Pool: pool}, Agents: &agents.Store{Pool: pool}, MailboxEnabled: mailboxConfig.Enabled}).Register(server)
		if mailboxConfig.Enabled && mailboxConfig.TasksEnabled {
			(&taskweb.Web{Browser: web, Tasks: &tasks.Store{Pool: pool}}).Register(server)
		}
	}
	if mailboxConfig.Enabled {
		store, err := mailbox.New(pool, mailboxConfig.CursorKey)
		if err != nil {
			return fmt.Errorf("mailbox configuration is invalid")
		}
		handler := mcpserver.New(store, mailboxConfig.AllowedOrigins)
		if mailboxConfig.TasksEnabled {
			handler = mcpserver.NewWithTasks(store, mailboxConfig.AllowedOrigins)
		}
		server.Handle("/mcp", handler)
	}
	grpcFailure := make(chan error, 1)
	if runnerIdentities != nil {
		if os.Getenv("CONTROL_PROBE_TOKEN") != "" {
			return errors.New("identity service cannot share diagnostic credentials")
		}
		pair, err := tls.X509KeyPair([]byte(os.Getenv("CONTROL_TLS_CERT")), []byte(os.Getenv("CONTROL_TLS_KEY")))
		if err != nil {
			return errors.New("invalid control TLS configuration")
		}
		rpc, err := control.New(runnerIdentities.Authenticate, &orchestration.Backend{Identities: runnerIdentities, Events: &controlevents.Store{Pool: pool, Identities: runnerIdentities}, Council: &councilstore.Store{Pool: pool, Identities: runnerIdentities}}, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})))
		if err != nil {
			return err
		}
		nativeListener, err := net.Listen("tcp", "0.0.0.0:9090")
		if err != nil {
			return errors.New("control listener unavailable")
		}
		defer rpc.Stop()
		defer nativeListener.Close()
		go func() {
			if err := rpc.Serve(nativeListener); err != nil && ctx.Err() == nil {
				grpcFailure <- errors.New("control serving failed")
				stop()
			}
		}()
	} else if token := os.Getenv("CONTROL_PROBE_TOKEN"); token != "" {
		cert, key := os.Getenv("CONTROL_TLS_CERT"), os.Getenv("CONTROL_TLS_KEY")
		if cert != "" || key != "" {
			pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
			if err != nil {
				return errors.New("invalid control TLS configuration")
			}
			rpc, err := controlprobe.New(token, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})))
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", "0.0.0.0:9090")
			if err != nil {
				return errors.New("control listener unavailable")
			}
			defer rpc.Stop()
			defer listener.Close()
			go func() {
				if err := rpc.Serve(listener); err != nil && ctx.Err() == nil {
					grpcFailure <- errors.New("control serving failed")
					stop()
				}
			}()
		} else {
			handler, shutdown, err := controlprobe.Wrap(server.HTTP.Handler, token)
			if err != nil {
				return err
			}
			defer shutdown()
			server.HTTP.Handler = handler
		}
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return fmt.Errorf("HTTP listener could not be opened")
	}
	logger.Info("server_started", "port", cfg.Port)
	err = server.Serve(ctx, listener)
	select {
	case grpcErr := <-grpcFailure:
		return grpcErr
	default:
		return err
	}
}
