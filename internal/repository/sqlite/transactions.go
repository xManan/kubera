package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
	"kubera/internal/repository"
)

const txColumns = `id, direction, amount_minor, currency, occurred_at, category_id,
	description, original_notification, reference_id, created_at, updated_at,
	voided_at, void_reason`

type txRepo struct{}

// NewTransactionRepository returns the SQLite-backed transaction repository.
func NewTransactionRepository() repository.TransactionRepository { return txRepo{} }

func scanTx(row interface{ Scan(...any) error }) (domain.Transaction, error) {
	var (
		t          domain.Transaction
		direction  string
		currency   string
		occurredAt string
		createdAt  string
		updatedAt  string
		refID      sql.NullString
		voidedAt   sql.NullString
		voidReason sql.NullString
	)
	err := row.Scan(&t.ID, &direction, &t.AmountMinor, &currency, &occurredAt, &t.CategoryID,
		&t.Description, &t.OriginalNotification, &refID, &createdAt, &updatedAt,
		&voidedAt, &voidReason)
	if errors.Is(err, sql.ErrNoRows) {
		return t, domain.NewError(domain.CodeTransactionNotFound, "The requested transaction does not exist.")
	}
	if err != nil {
		return t, fmt.Errorf("scan transaction: %w", err)
	}
	t.Direction = domain.Direction(direction)
	t.Currency = domain.CurrencyCode(currency)
	t.ReferenceID = strPtr(refID)
	t.VoidReason = strPtr(voidReason)
	if t.OccurredAt, err = parseTime(occurredAt); err != nil {
		return t, fmt.Errorf("scan transaction occurred_at: %w", err)
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return t, fmt.Errorf("scan transaction created_at: %w", err)
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return t, fmt.Errorf("scan transaction updated_at: %w", err)
	}
	if voidedAt.Valid {
		v, err := parseTime(voidedAt.String)
		if err != nil {
			return t, fmt.Errorf("scan transaction voided_at: %w", err)
		}
		t.VoidedAt = &v
	}
	return t, nil
}

func (txRepo) Insert(ctx context.Context, tx *sql.Tx, t domain.Transaction) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO transactions (
		id, direction, amount_minor, currency, occurred_at, category_id,
		description, original_notification, reference_id, normalized_description,
		created_at, updated_at, voided_at, void_reason
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(t.ID), string(t.Direction), t.AmountMinor, string(t.Currency),
		timeString(t.OccurredAt), string(t.CategoryID),
		t.Description, t.OriginalNotification, nullString(t.ReferenceID),
		duplicate.NormalizeText(t.Description),
		timeString(t.CreatedAt), timeString(t.UpdatedAt),
		nil, nil,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	return nil
}

func (txRepo) Get(ctx context.Context, q repository.Queryer, id domain.TransactionID) (domain.Transaction, error) {
	row := q.QueryRowContext(ctx,
		"SELECT "+txColumns+" FROM transactions WHERE id = ?", string(id))
	return scanTx(row)
}

// txCursor is the deterministic keyset cursor: last row (occurred_at, id).
type txCursor struct {
	OccurredAt string `json:"o"`
	ID         string `json:"i"`
}

func encodeCursor(t time.Time, id string) string {
	b, _ := json.Marshal(txCursor{OccurredAt: timeString(t), ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (txCursor, error) {
	var c txCursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, domain.NewError(domain.CodeInvalidRequest, "The pagination cursor is invalid.")
	}
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" || c.OccurredAt == "" {
		return c, domain.NewError(domain.CodeInvalidRequest, "The pagination cursor is invalid.")
	}
	return c, nil
}

func (txRepo) List(ctx context.Context, q repository.Queryer, f domain.TransactionFilter) ([]domain.Transaction, domain.Page, error) {
	where := []string{"1 = 1"}
	var args []any

	if f.Start != nil {
		where = append(where, "occurred_at >= ?")
		args = append(args, timeString(*f.Start))
	}
	if f.End != nil {
		where = append(where, "occurred_at < ?")
		args = append(args, timeString(*f.End))
	}
	if f.Direction != "" {
		where = append(where, "direction = ?")
		args = append(args, string(f.Direction))
	}
	if f.CategoryID != "" {
		where = append(where, "category_id = ?")
		args = append(args, string(f.CategoryID))
	}
	if f.MinAmount != nil {
		where = append(where, "amount_minor >= ?")
		args = append(args, *f.MinAmount)
	}
	if f.MaxAmount != nil {
		where = append(where, "amount_minor <= ?")
		args = append(args, *f.MaxAmount)
	}
	if f.Text != "" {
		where = append(where, "instr(lower(description), ?) > 0")
		args = append(args, strings.ToLower(f.Text))
	}
	if f.ReferenceID != "" {
		where = append(where, "reference_id = ?")
		args = append(args, f.ReferenceID)
	}
	if !f.IncludeVoided {
		where = append(where, "voided_at IS NULL")
	}
	if f.Cursor != "" {
		c, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, domain.Page{}, err
		}
		where = append(where, "(occurred_at < ? OR (occurred_at = ? AND id < ?))")
		args = append(args, c.OccurredAt, c.OccurredAt, c.ID)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	qargs := append(args, limit+1)
	query := "SELECT " + txColumns + " FROM transactions WHERE " +
		strings.Join(where, " AND ") +
		" ORDER BY occurred_at DESC, id DESC LIMIT ?"
	rows, err := q.QueryContext(ctx, query, qargs...)
	if err != nil {
		return nil, domain.Page{}, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, domain.Page{}, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, domain.Page{}, fmt.Errorf("list transactions: %w", err)
	}

	page := domain.Page{HasMore: len(out) > limit}
	if page.HasMore {
		out = out[:limit]
		last := out[len(out)-1]
		page.NextCursor = encodeCursor(last.OccurredAt, string(last.ID))
	}
	return out, page, nil
}

func (txRepo) Update(ctx context.Context, tx *sql.Tx, t domain.Transaction) error {
	res, err := tx.ExecContext(ctx, `UPDATE transactions SET
		direction = ?, amount_minor = ?, currency = ?, occurred_at = ?,
		category_id = ?, description = ?, reference_id = ?,
		normalized_description = ?, updated_at = ?
		WHERE id = ? AND voided_at IS NULL`,
		string(t.Direction), t.AmountMinor, string(t.Currency), timeString(t.OccurredAt),
		string(t.CategoryID), t.Description, nullString(t.ReferenceID),
		duplicate.NormalizeText(t.Description), timeString(t.UpdatedAt),
		string(t.ID),
	)
	if err != nil {
		return fmt.Errorf("update transaction: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NewError(domain.CodeTransactionNotFound, "The requested transaction does not exist.")
	}
	return nil
}

func (txRepo) Void(ctx context.Context, tx *sql.Tx, id domain.TransactionID, at time.Time, reason string) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE transactions SET voided_at = ?, void_reason = ?, updated_at = ? WHERE id = ? AND voided_at IS NULL",
		timeString(at), nullString(reason), timeString(at), string(id))
	if err != nil {
		return fmt.Errorf("void transaction: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NewError(domain.CodeTransactionNotFound, "The requested transaction does not exist.")
	}
	return nil
}

// FindLikelyDuplicate applies the documented matching priority: reference ID
// first, then the amount/currency/description/time combination within the
// tolerance window. Voided transactions are excluded. Matching runs inside
// the caller's write transaction so concurrent identical creates serialize.
func (txRepo) FindLikelyDuplicate(ctx context.Context, q repository.Queryer, c duplicate.Candidate, tolerance time.Duration) (domain.Transaction, bool, error) {
	if c.ReferenceID != "" {
		row := q.QueryRowContext(ctx,
			"SELECT "+txColumns+" FROM transactions WHERE voided_at IS NULL AND reference_id = ? AND currency = ? AND direction = ? ORDER BY occurred_at DESC",
			c.ReferenceID, string(c.Currency), string(c.Direction))
		t, err := scanTx(row)
		if err == nil {
			return t, true, nil
		}
		if !domain.Is(err, domain.CodeTransactionNotFound) {
			return t, false, err
		}
	}

	lo := timeString(c.OccurredAt.Add(-tolerance))
	hi := timeString(c.OccurredAt.Add(tolerance)) // half-open: [lo, hi)
	if lo == hi {
		hi = timeString(c.OccurredAt.Add(tolerance + time.Second))
	}
	row := q.QueryRowContext(ctx,
		"SELECT "+txColumns+" FROM transactions WHERE voided_at IS NULL AND direction = ? AND amount_minor = ? AND currency = ? AND normalized_description = ? AND occurred_at >= ? AND occurred_at < ? ORDER BY occurred_at DESC",
		string(c.Direction), c.AmountMinor, string(c.Currency), c.NormalizedDescription, lo, hi)
	t, err := scanTx(row)
	if err == nil {
		return t, true, nil
	}
	if domain.Is(err, domain.CodeTransactionNotFound) {
		return domain.Transaction{}, false, nil
	}
	return domain.Transaction{}, false, err
}
