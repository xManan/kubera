// Package mcp exposes Kubera's MCP tool surface: request decoding, response
// encoding, and stable error mapping. It contains no SQL or financial rules.
package mcp

import (
	"time"

	"kubera/internal/domain"
)

// tzRFC3339 renders timestamps as RFC 3339 UTC with an explicit Z offset.
func tzRFC3339(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// TransactionJSON is the canonical wire shape (see requirements doc section 5).
type TransactionJSON struct {
	ID           string  `json:"id"`
	Direction    string  `json:"direction"`
	AmountMinor  int64   `json:"amount_minor"`
	Currency     string  `json:"currency"`
	OccurredAt   string  `json:"occurred_at"`
	CategoryID   string  `json:"category_id"`
	Description  string  `json:"description"`
	ReferenceID  *string `json:"reference_id"`
	OriginalText string  `json:"original_text"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	VoidedAt     *string `json:"voided_at"`
	VoidReason   *string `json:"void_reason"`
}

func toTxJSON(t domain.Transaction) TransactionJSON {
	str := func(s string) *string {
		if s == "" {
			return nil
		}
		v := s
		return &v
	}
	var voidedAt *string
	if t.VoidedAt != nil {
		voidedAt = str(tzRFC3339(*t.VoidedAt))
	}
	return TransactionJSON{
		ID:           string(t.ID),
		Direction:    string(t.Direction),
		AmountMinor:  t.AmountMinor,
		Currency:     string(t.Currency),
		OccurredAt:   tzRFC3339(t.OccurredAt),
		CategoryID:   string(t.CategoryID),
		Description:  t.Description,
		ReferenceID:  str(t.ReferenceID),
		OriginalText: t.OriginalNotification,
		CreatedAt:    tzRFC3339(t.CreatedAt),
		UpdatedAt:    tzRFC3339(t.UpdatedAt),
		VoidedAt:     voidedAt,
		VoidReason:   str(t.VoidReason),
	}
}

type CategoryJSON struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	ArchivedAt *string `json:"archived_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

func toCatJSON(c domain.Category) CategoryJSON {
	var archived *string
	if c.ArchivedAt != nil {
		s := tzRFC3339(*c.ArchivedAt)
		archived = &s
	}
	return CategoryJSON{
		ID:         string(c.ID),
		Name:       c.Name,
		ArchivedAt: archived,
		CreatedAt:  tzRFC3339(c.CreatedAt),
		UpdatedAt:  tzRFC3339(c.UpdatedAt),
	}
}

// SummaryJSON mirrors domain.Summary with stable field names.
type SummaryJSON struct {
	Start               string            `json:"start"`
	End                 string            `json:"end"`
	CurrencyTotals      []CurrencyTotalJ  `json:"currency_totals"`
	Categories          []CategoryTotalJ  `json:"categories"`
	LargestTransactions []TransactionJSON `json:"largest_transactions,omitempty"`
}

type CurrencyTotalJ struct {
	Currency         string `json:"currency"`
	MoneyInMinor     int64  `json:"money_in_minor"`
	MoneyOutMinor    int64  `json:"money_out_minor"`
	NetMinor         int64  `json:"net_minor"`
	TransactionCount int64  `json:"transaction_count"`
}

type CategoryTotalJ struct {
	CategoryID       string `json:"category_id"`
	CategoryName     string `json:"category_name"`
	Currency         string `json:"currency"`
	Direction        string `json:"direction"`
	TotalMinor       int64  `json:"total_minor"`
	TransactionCount int64  `json:"transaction_count"`
}

func toSummaryJSON(s domain.Summary) SummaryJSON {
	out := SummaryJSON{
		Start: s.StartRFC3339, End: s.EndRFC3339,
		CurrencyTotals: []CurrencyTotalJ{},
		Categories:     []CategoryTotalJ{},
	}
	for _, t := range s.CurrencyTotals {
		out.CurrencyTotals = append(out.CurrencyTotals, CurrencyTotalJ{
			Currency: string(t.Currency), MoneyInMinor: t.MoneyInMinor,
			MoneyOutMinor: t.MoneyOutMinor, NetMinor: t.NetMinor,
			TransactionCount: t.TransactionCount,
		})
	}
	for _, c := range s.Categories {
		out.Categories = append(out.Categories, CategoryTotalJ{
			CategoryID: string(c.CategoryID), CategoryName: c.CategoryName,
			Currency: string(c.Currency), Direction: string(c.Direction),
			TotalMinor: c.TotalMinor, TransactionCount: c.TransactionCount,
		})
	}
	for _, t := range s.LargestTransactions {
		out.LargestTransactions = append(out.LargestTransactions, toTxJSON(t))
	}
	return out
}
