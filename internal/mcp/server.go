package mcp

import (
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/app"
	"kubera/internal/domain"
)

// New builds the Kubera MCP server with all v1 tools registered.
func New(svc *app.Services, log *slog.Logger, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "kubera", Version: version}, nil)
	h := &Handler{Svc: svc, Log: log}
	h.registerTxTools(server)
	h.registerCategoryTools(server)
	h.registerReportTools(server)
	return server
}

// parseDate parses a UTC calendar date string (YYYY-MM-DD).
func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, domain.NewError(domain.CodeInvalidDateRange,
			"Date must be in YYYY-MM-DD format, interpreted in UTC.")
	}
	return t.UTC(), nil
}
