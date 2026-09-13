package mcp

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/app"
	"kubera/internal/domain"
)

// Handler carries the services and logger shared by all tool handlers.
type Handler struct {
	Svc *app.Services
	Log *slog.Logger
}

// registerTxTools registers the transaction tools on the server.
func (h *Handler) registerTxTools(s *mcp.Server) {
	mcp.AddTool[CreateTransactionIn, any](s, &mcp.Tool{
		Name:        "create_transaction",
		Description: "Create a money_in or money_out transaction. Amounts are positive integer minor units (e.g. paise). Timestamps are RFC 3339 UTC. Detects likely duplicates before writing.",
	}, logTool(h, "create_transaction", h.CreateTransaction))

	mcp.AddTool[GetTransactionIn, any](s, &mcp.Tool{
		Name:        "get_transaction",
		Description: "Get a single transaction by ID, including void fields when present.",
	}, logTool(h, "get_transaction", h.GetTransaction))

	mcp.AddTool[ListTransactionsIn, any](s, &mcp.Tool{
		Name:        "list_transactions",
		Description: "List transactions with filters and deterministic keyset pagination. Half-open UTC range: start <= occurred_at < end.",
	}, logTool(h, "list_transactions", h.ListTransactions))

	mcp.AddTool[UpdateTransactionIn, any](s, &mcp.Tool{
		Name:        "update_transaction",
		Description: "Correct a saved transaction. Provide at least one mutable field. The transaction ID and original notification text are preserved; original_notification is not mutable. Send reference_id as an empty string to clear it.",
	}, logTool(h, "update_transaction", h.UpdateTransaction))

	mcp.AddTool[VoidTransactionIn, any](s, &mcp.Tool{
		Name:        "void_transaction",
		Description: "Void a transaction. Nothing is deleted; voided transactions are excluded from reports by default and remain retrievable by ID.",
	}, logTool(h, "void_transaction", h.VoidTransaction))
}

type CreateTransactionIn struct {
	Direction            string `json:"direction" jsonschema:"money_in or money_out"`
	AmountMinor          int64  `json:"amount_minor" jsonschema:"positive integer amount in minor units, e.g. 19800 for Rs.198"`
	Currency             string `json:"currency" jsonschema:"three-letter uppercase ISO 4217 code, e.g. INR"`
	OccurredAt           string `json:"occurred_at" jsonschema:"RFC 3339 UTC timestamp with explicit offset, e.g. 2026-01-15T10:30:00Z"`
	CategoryID           string `json:"category_id" jsonschema:"ID of an active category"`
	Description          string `json:"description,omitempty" jsonschema:"merchant or counterparty, max 500 characters"`
	OriginalNotification string `json:"original_notification,omitempty" jsonschema:"original notification text, preserved verbatim, max 10000 characters"`
	ReferenceID          string `json:"reference_id,omitempty" jsonschema:"optional bank or payment reference ID, max 256 characters"`
}

func (h *Handler) CreateTransaction(ctx context.Context, req *mcp.CallToolRequest, in CreateTransactionIn) (*mcp.CallToolResult, any, error) {
	occurredAt, err := domain.ParseTimestamp(in.OccurredAt)
	if err != nil {
		return toolErr(err, h.Log)
	}
	res, err := h.Svc.CreateTransaction(ctx, app.CreateTransactionInput{
		Direction:            domain.Direction(in.Direction),
		AmountMinor:          in.AmountMinor,
		Currency:             domain.CurrencyCode(in.Currency),
		OccurredAt:           occurredAt,
		CategoryID:           domain.CategoryID(in.CategoryID),
		Description:          in.Description,
		OriginalNotification: in.OriginalNotification,
		ReferenceID:          in.ReferenceID,
	})
	if err != nil {
		return toolErr(err, h.Log)
	}
	if res.Status == "duplicate" {
		return toolOK(map[string]any{
			"status":               "duplicate",
			"existing_transaction": toTxJSON(res.Duplicate.Existing),
			"match_reason":         res.Duplicate.Reason,
		})
	}
	return toolOK(map[string]any{"status": "created", "transaction": toTxJSON(res.Transaction)})
}

type GetTransactionIn struct {
	TransactionID string `json:"transaction_id"`
}

func (h *Handler) GetTransaction(ctx context.Context, req *mcp.CallToolRequest, in GetTransactionIn) (*mcp.CallToolResult, any, error) {
	t, err := h.Svc.GetTransaction(ctx, domain.TransactionID(in.TransactionID))
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(map[string]any{"transaction": toTxJSON(t)})
}

type ListTransactionsIn struct {
	Start         string `json:"start,omitempty" jsonschema:"RFC 3339 UTC range start"`
	End           string `json:"end,omitempty" jsonschema:"RFC 3339 UTC range end (exclusive)"`
	Direction     string `json:"direction,omitempty" jsonschema:"filter by money_in or money_out"`
	CategoryID    string `json:"category_id,omitempty"`
	MinAmount     *int64 `json:"min_amount_minor,omitempty"`
	MaxAmount     *int64 `json:"max_amount_minor,omitempty"`
	Text          string `json:"text,omitempty" jsonschema:"substring filter on description"`
	ReferenceID   string `json:"reference_id,omitempty"`
	IncludeVoided bool   `json:"include_voided,omitempty"`
	Limit         int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 100"`
	Cursor        string `json:"cursor,omitempty" jsonschema:"opaque cursor from a previous page"`
}

func (h *Handler) ListTransactions(ctx context.Context, req *mcp.CallToolRequest, in ListTransactionsIn) (*mcp.CallToolResult, any, error) {
	f := domain.TransactionFilter{
		Direction:     domain.Direction(in.Direction),
		CategoryID:    domain.CategoryID(in.CategoryID),
		Text:          in.Text,
		ReferenceID:   in.ReferenceID,
		IncludeVoided: in.IncludeVoided,
		Limit:         in.Limit,
		Cursor:        in.Cursor,
	}
	if f.Limit > maxPageSize {
		f.Limit = maxPageSize
	}
	if in.Start != "" {
		t, err := domain.ParseTimestamp(in.Start)
		if err != nil {
			return toolErr(err, h.Log)
		}
		f.Start = &t
	}
	if in.End != "" {
		t, err := domain.ParseTimestamp(in.End)
		if err != nil {
			return toolErr(err, h.Log)
		}
		f.End = &t
	}
	txs, page, err := h.Svc.ListTransactions(ctx, f)
	if err != nil {
		return toolErr(err, h.Log)
	}
	txsJSON := make([]TransactionJSON, 0, len(txs))
	for _, t := range txs {
		txsJSON = append(txsJSON, toTxJSON(t))
	}
	payload := map[string]any{"transactions": txsJSON, "has_more": page.HasMore}
	if page.NextCursor != "" {
		payload["next_cursor"] = page.NextCursor
	} else {
		payload["next_cursor"] = nil
	}
	return toolOK(payload)
}

type UpdateTransactionIn struct {
	TransactionID string  `json:"transaction_id"`
	Direction     *string `json:"direction,omitempty" jsonschema:"money_in or money_out"`
	AmountMinor   *int64  `json:"amount_minor,omitempty"`
	Currency      *string `json:"currency,omitempty"`
	OccurredAt    *string `json:"occurred_at,omitempty" jsonschema:"RFC 3339 UTC timestamp with explicit offset"`
	CategoryID    *string `json:"category_id,omitempty"`
	Description   *string `json:"description,omitempty"`
	ReferenceID   *string `json:"reference_id,omitempty" jsonschema:"empty string clears the reference"`
}

func (h *Handler) UpdateTransaction(ctx context.Context, req *mcp.CallToolRequest, in UpdateTransactionIn) (*mcp.CallToolResult, any, error) {
	patch := app.UpdateTransactionInput{ID: domain.TransactionID(in.TransactionID)}
	if in.Direction != nil {
		patch.Direction = ptr(domain.Direction(*in.Direction))
	}
	if in.AmountMinor != nil {
		patch.AmountMinor = in.AmountMinor
	}
	if in.Currency != nil {
		patch.Currency = ptr(domain.CurrencyCode(*in.Currency))
	}
	if in.OccurredAt != nil {
		t, err := domain.ParseTimestamp(*in.OccurredAt)
		if err != nil {
			return toolErr(err, h.Log)
		}
		patch.OccurredAt = &t
	}
	if in.CategoryID != nil {
		patch.CategoryID = ptr(domain.CategoryID(*in.CategoryID))
	}
	if in.Description != nil {
		patch.Description = in.Description
	}
	if in.ReferenceID != nil {
		patch.ReferenceID = in.ReferenceID
	}
	t, err := h.Svc.UpdateTransaction(ctx, patch)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(map[string]any{"transaction": toTxJSON(t)})
}

type VoidTransactionIn struct {
	TransactionID string `json:"transaction_id"`
	Reason        string `json:"reason,omitempty" jsonschema:"optional reason, max 1000 characters"`
}

func (h *Handler) VoidTransaction(ctx context.Context, req *mcp.CallToolRequest, in VoidTransactionIn) (*mcp.CallToolResult, any, error) {
	t, err := h.Svc.VoidTransaction(ctx, domain.TransactionID(in.TransactionID), in.Reason)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(map[string]any{"status": "voided", "transaction": toTxJSON(t)})
}

// logTool wraps a typed handler with outcome and duration logging.
func logTool[In any](h *Handler, tool string, fn func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		start := time.Now()
		res, out, err := fn(ctx, req, in)
		outcome := "ok"
		if res != nil && res.IsError {
			outcome = "error"
		}
		h.Log.Info("tool call",
			"tool", tool,
			"duration_ms", time.Since(start).Milliseconds(),
			"outcome", outcome,
		)
		return res, out, err
	}
}

func ptr[T any](v T) *T { return &v }

const maxPageSize = 100
