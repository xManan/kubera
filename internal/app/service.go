// Package app contains Kubera's business rules, between the MCP layer and the
// repository layer. All multi-record writes happen atomically here.
package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
	"kubera/internal/repository"
)

// Services wires repositories and shared behavior. One instance per process.
type Services struct {
	DB *sql.DB

	Transactions repository.TransactionRepository
	Categories   repository.CategoryRepository
	Audits       repository.AuditRepository
	Reports      repository.ReportRepository

	// Now and NewID are indirections for deterministic tests.
	Now   func() time.Time
	NewID func(prefix string) string
}

func newID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// now and newID are nil-safe accessors over the test indirections.
func (s *Services) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Services) id(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return newID(prefix)
}

// withWriteTx runs fn inside a single SQLite write transaction. The DSN
// configures BEGIN IMMEDIATE, so the final duplicate check and insert
// serialize against concurrent writers.
func (s *Services) withWriteTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.NewError(domain.CodeDatabaseUnavailable, "The database is unavailable.")
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// rfc3339 canonicalizes a timestamp for audit payloads and API output.
func rfc3339(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func voidString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return rfc3339(*t)
}

// auditSnapshot marshals the audit-safe view of a transaction (no original
// notification text).
func auditSnapshot(t domain.Transaction) json.RawMessage {
	b, err := json.Marshal(domain.AuditSnapshot{
		ID: t.ID, Direction: t.Direction, AmountMinor: t.AmountMinor,
		Currency: t.Currency, OccurredAt: rfc3339(t.OccurredAt),
		CategoryID: t.CategoryID, Description: t.Description,
		ReferenceID: t.ReferenceID, VoidedAt: voidString(t.VoidedAt),
	})
	if err != nil {
		return nil
	}
	return b
}

// toCandidate builds the duplicate-evaluation input from a transaction.
func toCandidate(t domain.Transaction) duplicate.Candidate {
	return duplicate.Candidate{
		Direction:             t.Direction,
		AmountMinor:           t.AmountMinor,
		Currency:              t.Currency,
		OccurredAt:            t.OccurredAt,
		NormalizedDescription: duplicate.NormalizeText(t.Description),
		ReferenceID:           duplicate.NormalizeReference(t.ReferenceID),
	}
}
