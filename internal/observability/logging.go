// Package observability provides structured logging setup.
package observability

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger returns a JSON slog.Logger on stderr (stdout carries the MCP
// stdio protocol). Notification text and request payloads are never logged.
func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
