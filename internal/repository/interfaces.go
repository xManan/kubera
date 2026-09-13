// Package repository defines the data-access interfaces used by the service
// layer. SQL lives only in the sqlite subpackage.
package repository

import (
	"context"
	"database/sql"
	"time"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
)

// Queryer is implemented by both *sql.DB and *sql.Tx, letting read methods
// run inside or outside a transaction.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// WriteTx is a transaction handle passed to write methods.
type WriteTx = *sql.Tx

type TransactionRepository interface {
	Insert(ctx context.Context, tx WriteTx, t domain.Transaction) error
	Get(ctx context.Context, q Queryer, id domain.TransactionID) (domain.Transaction, error)
	List(ctx context.Context, q Queryer, filter domain.TransactionFilter) ([]domain.Transaction, domain.Page, error)
	Update(ctx context.Context, tx WriteTx, t domain.Transaction) error
	Void(ctx context.Context, tx WriteTx, id domain.TransactionID, at time.Time, reason string) error
	FindLikelyDuplicate(ctx context.Context, q Queryer, c duplicate.Candidate, tolerance time.Duration) (domain.Transaction, bool, error)
}

type CategoryRepository interface {
	Create(ctx context.Context, tx WriteTx, c domain.Category) error
	Get(ctx context.Context, q Queryer, id domain.CategoryID) (domain.Category, error)
	FindActiveByName(ctx context.Context, q Queryer, nameNormalized string) (domain.Category, bool, error)
	List(ctx context.Context, q Queryer, includeArchived bool) ([]domain.Category, error)
	Update(ctx context.Context, tx WriteTx, c domain.Category) error
	Archive(ctx context.Context, tx WriteTx, id domain.CategoryID, at time.Time) error
}

type AuditRepository interface {
	Append(ctx context.Context, tx WriteTx, e domain.AuditEvent) error
	ListForEntity(ctx context.Context, q Queryer, entityType, entityID string) ([]domain.AuditEvent, error)
}

type ReportRepository interface {
	CurrencyTotals(ctx context.Context, q Queryer, start, end time.Time) ([]domain.CurrencyTotal, error)
	CategoryTotals(ctx context.Context, q Queryer, start, end time.Time) ([]domain.CategoryTotal, error)
	LargestTransactions(ctx context.Context, q Queryer, start, end time.Time, limit int) ([]domain.Transaction, error)
}
