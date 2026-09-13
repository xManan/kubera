package domain

import "time"

// Explicit domain primitives. Raw strings are not passed between layers.
type (
	TransactionID string
	CategoryID    string
	AuditEventID  string
	CurrencyCode  string
	Direction     string
)

const (
	MoneyIn  Direction = "money_in"
	MoneyOut Direction = "money_out"
)

// Transaction is the canonical financial record. AmountMinor is a positive
// integer in minor units (never floating point); direction determines in/out.
type Transaction struct {
	ID                   TransactionID
	Direction            Direction
	AmountMinor          int64
	Currency             CurrencyCode
	OccurredAt           time.Time
	CategoryID           CategoryID
	Description          string
	OriginalNotification string
	ReferenceID          string // empty when absent
	CreatedAt            time.Time
	UpdatedAt            time.Time
	VoidedAt             *time.Time
	VoidReason           string // empty when absent
}

func (t Transaction) IsVoided() bool { return t.VoidedAt != nil }

// AuditSnapshot is the JSON payload stored in audit events. It deliberately
// omits original notification text.
type AuditSnapshot struct {
	ID          TransactionID `json:"id"`
	Direction   Direction     `json:"direction"`
	AmountMinor int64         `json:"amount_minor"`
	Currency    CurrencyCode  `json:"currency"`
	OccurredAt  string        `json:"occurred_at"`
	CategoryID  CategoryID    `json:"category_id"`
	Description string        `json:"description,omitempty"`
	ReferenceID string        `json:"reference_id,omitempty"`
	VoidedAt    string        `json:"voided_at,omitempty"`
}
