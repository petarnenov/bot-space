package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/petarnenov/bot-space/internal/config"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/httpserver"
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
	if len(os.Args) == 2 {
		command = os.Args[1]
	}
	if len(os.Args) > 2 || (command != "serve" && command != "migrate") {
		return fmt.Errorf("usage: mailbox [serve|migrate]")
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
	server := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, versions) }, logger)
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		return fmt.Errorf("HTTP listener could not be opened")
	}
	logger.Info("server_started", "port", cfg.Port)
	return server.Serve(ctx, listener)
}
