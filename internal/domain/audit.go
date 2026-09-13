package domain

import (
	"encoding/json"
	"time"
)

// AuditEvent is an append-only record of a data change.
type AuditEvent struct {
	ID         AuditEventID
	EntityType string // "transaction"
	EntityID   string
	Operation  string // "created" | "updated" | "voided"
	OccurredAt time.Time
	Previous   json.RawMessage // nil when not applicable
	Current    json.RawMessage // nil when not applicable
	Reason     string          // empty when absent
}

const (
	EntityTypeTransaction = "transaction"

	OpCreated = "created"
	OpUpdated = "updated"
	OpVoided  = "voided"
)
