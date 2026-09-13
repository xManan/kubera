package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/domain"
)

func (h *Handler) registerReportTools(s *mcp.Server) {
	mcp.AddTool[DailySummaryIn, any](s, &mcp.Tool{
		Name:        "get_daily_summary",
		Description: "Daily summary for a UTC date (money in, money out, net, counts, category breakdowns, largest transactions).",
	}, logTool(h, "get_daily_summary", h.DailySummary))

	mcp.AddTool[MonthlySummaryIn, any](s, &mcp.Tool{
		Name:        "get_monthly_summary",
		Description: "Monthly summary for a UTC calendar month (money in, money out, net, counts, category breakdowns, largest transactions).",
	}, logTool(h, "get_monthly_summary", h.MonthlySummary))

	mcp.AddTool[TransactionSummaryIn, any](s, &mcp.Tool{
		Name:        "get_transaction_summary",
		Description: "Summary over an arbitrary half-open UTC range: start <= occurred_at < end. Different currencies are never summed together.",
	}, logTool(h, "get_transaction_summary", h.TransactionSummary))

	mcp.AddTool[CategoryBreakdownIn, any](s, &mcp.Tool{
		Name:        "get_category_breakdown",
		Description: "Category breakdown over a half-open UTC range, grouped by category, currency, and direction.",
	}, logTool(h, "get_category_breakdown", h.CategoryBreakdown))
}

type DailySummaryIn struct {
	Date string `json:"date" jsonschema:"UTC calendar date, e.g. 2026-01-15"`
}

func (h *Handler) DailySummary(ctx context.Context, req *mcp.CallToolRequest, in DailySummaryIn) (*mcp.CallToolResult, any, error) {
	date, err := parseDate(in.Date)
	if err != nil {
		return toolErr(err, h.Log)
	}
	sum, err := h.Svc.DailySummary(ctx, date)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(toSummaryJSON(sum))
}

type MonthlySummaryIn struct {
	Year  int `json:"year" jsonschema:"calendar year, e.g. 2026"`
	Month int `json:"month" jsonschema:"calendar month 1-12, interpreted in UTC"`
}

func (h *Handler) MonthlySummary(ctx context.Context, req *mcp.CallToolRequest, in MonthlySummaryIn) (*mcp.CallToolResult, any, error) {
	sum, err := h.Svc.MonthlySummary(ctx, in.Year, in.Month)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(toSummaryJSON(sum))
}

type TransactionSummaryIn struct {
	Start string `json:"start" jsonschema:"RFC 3339 UTC range start (inclusive)"`
	End   string `json:"end" jsonschema:"RFC 3339 UTC range end (exclusive)"`
}

func (h *Handler) TransactionSummary(ctx context.Context, req *mcp.CallToolRequest, in TransactionSummaryIn) (*mcp.CallToolResult, any, error) {
	start, err := domain.ParseTimestamp(in.Start)
	if err != nil {
		return toolErr(err, h.Log)
	}
	end, err := domain.ParseTimestamp(in.End)
	if err != nil {
		return toolErr(err, h.Log)
	}
	sum, err := h.Svc.Summary(ctx, start, end, false)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(toSummaryJSON(sum))
}

type CategoryBreakdownIn struct {
	Start string `json:"start" jsonschema:"RFC 3339 UTC range start (inclusive)"`
	End   string `json:"end" jsonschema:"RFC 3339 UTC range end (exclusive)"`
}

func (h *Handler) CategoryBreakdown(ctx context.Context, req *mcp.CallToolRequest, in CategoryBreakdownIn) (*mcp.CallToolResult, any, error) {
	start, err := domain.ParseTimestamp(in.Start)
	if err != nil {
		return toolErr(err, h.Log)
	}
	end, err := domain.ParseTimestamp(in.End)
	if err != nil {
		return toolErr(err, h.Log)
	}
	sum, err := h.Svc.CategoryBreakdown(ctx, start, end)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(toSummaryJSON(sum))
}
