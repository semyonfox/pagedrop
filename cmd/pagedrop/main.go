package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/semyonfox/pagedrop/internal/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pagedrop:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(app.Version())
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] == "upload" {
		return app.UploadCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "replace" {
		return app.ReplaceCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "list" {
		return app.ListCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "stats" {
		return app.StatsCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "info" {
		return app.InfoCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "configure" {
		return app.ConfigureCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] == "delete" {
		return app.DeleteCLI(os.Args[2:])
	}
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		return fmt.Errorf("usage: pagedrop [serve|configure|upload|list|stats|info|replace|delete]")
	}

	cfg, err := app.ConfigFromEnv()
	if err != nil {
		return err
	}
	server, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	slog.Info("starting PageDrop", "address", cfg.ListenAddr, "public_url", cfg.PublicBaseURL)
	return server.Serve(ctx)
}
