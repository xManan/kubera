// Command kubera is the Kubera v1 MCP server: a channel-agnostic
// personal-finance transaction tracker backed by SQLite.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/app"
	"kubera/internal/config"
	kubermcp "kubera/internal/mcp"
	"kubera/internal/observability"
	"kubera/internal/repository/sqlite"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "kubera: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := observability.NewLogger(cfg.LogLevel)
	log.Info("starting kubera", "version", version, "transport", cfg.Transport)

	if err := ensureParentDir(cfg.DatabasePath); err != nil {
		return err
	}

	db, err := sqlite.Open(cfg.DatabasePath, cfg.BusyTimeoutMS)
	if err != nil {
		return err
	}
	defer db.Close()

	if cfg.MigrateOnStart {
		v, err := sqlite.Migrate(context.Background(), db)
		if err != nil {
			return fmt.Errorf("migrations failed: %w", err)
		}
		log.Info("database ready", "schema_version", v)
	}

	svc := &app.Services{
		DB:           db,
		Transactions: sqlite.NewTransactionRepository(),
		Categories:   sqlite.NewCategoryRepository(),
		Audits:       sqlite.NewAuditRepository(),
		Reports:      sqlite.NewReportRepository(),
	}
	server := kubermcp.New(svc, log, version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("mcp server listening", "transport", "stdio")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		return err
	}
	log.Info("kubera stopped")
	return nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("database directory %q is not usable: %w", dir, err)
	}
	return nil
}
