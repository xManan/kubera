package app

import (
	"context"
	"time"

	"kubera/internal/domain"
)

// Summary returns totals, category breakdowns, and largest transactions for
// a half-open UTC range [start, end). Voided transactions are excluded.
func (s *Services) Summary(ctx context.Context, start, end time.Time, withLargest bool) (domain.Summary, error) {
	start, end = start.UTC(), end.UTC()
	if !end.After(start) {
		return domain.Summary{}, domain.NewError(domain.CodeInvalidDateRange,
			"The end of the date range must be after its start.")
	}
	if start.Year() < 1 || end.Year() > 9999 {
		return domain.Summary{}, domain.NewError(domain.CodeInvalidDateRange,
			"The date range is outside the supported range.")
	}

	sum := domain.Summary{
		Start: start, End: end,
		StartRFC3339: rfc3339(start), EndRFC3339: rfc3339(end),
	}
	var err error
	if sum.CurrencyTotals, err = s.Reports.CurrencyTotals(ctx, s.DB, start, end); err != nil {
		return domain.Summary{}, err
	}
	if sum.Categories, err = s.Reports.CategoryTotals(ctx, s.DB, start, end); err != nil {
		return domain.Summary{}, err
	}
	if withLargest {
		if sum.LargestTransactions, err = s.Reports.LargestTransactions(ctx, s.DB, start, end, 5); err != nil {
			return domain.Summary{}, err
		}
	}
	return sum, nil
}

// DailySummary summarizes the UTC calendar day containing date.
func (s *Services) DailySummary(ctx context.Context, date time.Time) (domain.Summary, error) {
	start := time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return s.Summary(ctx, start, start.Add(24*time.Hour), true)
}

// MonthlySummary summarizes the UTC calendar month (with largest transactions).
func (s *Services) MonthlySummary(ctx context.Context, year int, month int) (domain.Summary, error) {
	if month < 1 || month > 12 {
		return domain.Summary{}, domain.NewError(domain.CodeInvalidDateRange,
			"Month must be between 1 and 12.")
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	if start.Year() != year || int(start.Month()) != month {
		return domain.Summary{}, domain.NewError(domain.CodeInvalidDateRange,
			"The requested year/month is outside the supported range.")
	}
	return s.Summary(ctx, start, start.AddDate(0, 1, 0), true)
}

// CategoryBreakdown returns per-category (and currency and direction) totals
// for a half-open UTC range.
func (s *Services) CategoryBreakdown(ctx context.Context, start, end time.Time) (domain.Summary, error) {
	start, end = start.UTC(), end.UTC()
	if !end.After(start) {
		return domain.Summary{}, domain.NewError(domain.CodeInvalidDateRange,
			"The end of the date range must be after its start.")
	}
	sum := domain.Summary{Start: start, End: end, StartRFC3339: rfc3339(start), EndRFC3339: rfc3339(end)}
	var err error
	if sum.Categories, err = s.Reports.CategoryTotals(ctx, s.DB, start, end); err != nil {
		return domain.Summary{}, err
	}
	return sum, nil
}
