package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"kubera/internal/domain"
	"kubera/internal/repository"
)

type reportRepo struct{}

// NewReportRepository returns the SQLite-backed reporting repository. All
// report queries exclude voided transactions and use half-open UTC ranges.
func NewReportRepository() repository.ReportRepository { return reportRepo{} }

func (reportRepo) CurrencyTotals(ctx context.Context, q repository.Queryer, start, end time.Time) ([]domain.CurrencyTotal, error) {
	rows, err := q.QueryContext(ctx, `SELECT
			currency,
			SUM(CASE WHEN direction = 'money_in' THEN amount_minor ELSE 0 END),
			SUM(CASE WHEN direction = 'money_out' THEN amount_minor ELSE 0 END),
			COUNT(*)
		FROM transactions
		WHERE occurred_at >= ? AND occurred_at < ? AND voided_at IS NULL
		GROUP BY currency
		ORDER BY currency ASC`,
		timeString(start), timeString(end))
	if err != nil {
		return nil, fmt.Errorf("currency totals: %w", err)
	}
	defer rows.Close()
	var out []domain.CurrencyTotal
	for rows.Next() {
		var t domain.CurrencyTotal
		if err := rows.Scan(&t.Currency, &t.MoneyInMinor, &t.MoneyOutMinor, &t.TransactionCount); err != nil {
			return nil, fmt.Errorf("scan currency total: %w", err)
		}
		t.NetMinor = t.MoneyInMinor - t.MoneyOutMinor
		out = append(out, t)
	}
	return out, rows.Err()
}

func (reportRepo) CategoryTotals(ctx context.Context, q repository.Queryer, start, end time.Time) ([]domain.CategoryTotal, error) {
	rows, err := q.QueryContext(ctx, `SELECT
			t.category_id,
			c.name,
			t.currency,
			t.direction,
			SUM(t.amount_minor) AS total_minor,
			COUNT(*)
		FROM transactions t
		LEFT JOIN categories c ON c.id = t.category_id
		WHERE t.occurred_at >= ? AND t.occurred_at < ? AND t.voided_at IS NULL
		GROUP BY t.category_id, t.currency, t.direction
		ORDER BY t.currency ASC, t.direction ASC, total_minor DESC, t.category_id ASC`,
		timeString(start), timeString(end))
	if err != nil {
		return nil, fmt.Errorf("category totals: %w", err)
	}
	defer rows.Close()
	var out []domain.CategoryTotal
	for rows.Next() {
		var (
			t     domain.CategoryTotal
			name  sql.NullString
			total int64
		)
		if err := rows.Scan(&t.CategoryID, &name, &t.Currency, &t.Direction, &total, &t.TransactionCount); err != nil {
			return nil, fmt.Errorf("scan category total: %w", err)
		}
		t.TotalMinor = total
		t.CategoryName = name.String
		out = append(out, t)
	}
	return out, rows.Err()
}

func (reportRepo) LargestTransactions(ctx context.Context, q repository.Queryer, start, end time.Time, limit int) ([]domain.Transaction, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT "+txColumns+" FROM transactions WHERE occurred_at >= ? AND occurred_at < ? AND voided_at IS NULL ORDER BY amount_minor DESC, occurred_at ASC, id ASC LIMIT ?",
		timeString(start), timeString(end), limit)
	if err != nil {
		return nil, fmt.Errorf("largest transactions: %w", err)
	}
	defer rows.Close()
	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
