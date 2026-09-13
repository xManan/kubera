package domain

import "time"

// CurrencyTotal holds per-currency aggregate totals for a report range.
type CurrencyTotal struct {
	Currency         CurrencyCode `json:"currency"`
	MoneyInMinor     int64        `json:"money_in_minor"`
	MoneyOutMinor    int64        `json:"money_out_minor"`
	NetMinor         int64        `json:"net_minor"`
	TransactionCount int64        `json:"transaction_count"`
}

// CategoryTotal holds a per-category aggregate. Direction separates spending
// from income breakdowns; different currencies are never summed together.
type CategoryTotal struct {
	CategoryID       CategoryID   `json:"category_id"`
	CategoryName     string       `json:"category_name"`
	Currency         CurrencyCode `json:"currency"`
	Direction        Direction    `json:"direction"`
	TotalMinor       int64        `json:"total_minor"`
	TransactionCount int64        `json:"transaction_count"`
}

// Summary is the shared shape for daily, monthly, and custom-period reports.
type Summary struct {
	Start               time.Time       `json:"-"`
	End                 time.Time       `json:"-"`
	StartRFC3339        string          `json:"start"`
	EndRFC3339          string          `json:"end"`
	CurrencyTotals      []CurrencyTotal `json:"currency_totals"`
	Categories          []CategoryTotal `json:"categories"`
	LargestTransactions []Transaction   `json:"largest_transactions,omitempty"`
}

// TransactionFilter selects transactions for listing. Zero values mean
// "no filter". Start/End form a half-open range: start <= occurred_at < end.
type TransactionFilter struct {
	Start         *time.Time
	End           *time.Time
	Direction     Direction
	CategoryID    CategoryID
	MinAmount     *int64
	MaxAmount     *int64
	Text          string // substring match on description
	ReferenceID   string
	IncludeVoided bool
	Limit         int
	Cursor        string
}

// Page is pagination metadata with a deterministic keyset cursor.
type Page struct {
	NextCursor string
	HasMore    bool
}
