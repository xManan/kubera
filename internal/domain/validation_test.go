package domain

import (
	"errors"
	"testing"
	"time"
)

func TestValidateDirection(t *testing.T) {
	if err := ValidateDirection(MoneyIn); err != nil {
		t.Errorf("money_in should be valid: %v", err)
	}
	if err := ValidateDirection(MoneyOut); err != nil {
		t.Errorf("money_out should be valid: %v", err)
	}
	if err := ValidateDirection(Direction("in")); err == nil {
		t.Error("unknown direction should be rejected")
	}
}

func TestValidateAmount(t *testing.T) {
	if err := ValidateAmount(19800); err != nil {
		t.Errorf("19800 should be valid: %v", err)
	}
	for _, a := range []int64{0, -1} {
		if err := ValidateAmount(a); !Is(err, CodeInvalidAmount) {
			t.Errorf("amount %d should be invalid_amount, got %v", a, err)
		}
	}
}

func TestValidateCurrency(t *testing.T) {
	if err := ValidateCurrency("INR"); err != nil {
		t.Errorf("INR should be valid: %v", err)
	}
	for _, c := range []CurrencyCode{"inr", "IN", "INRX", "I R"} {
		if err := ValidateCurrency(c); !Is(err, CodeInvalidCurrency) {
			t.Errorf("currency %q should be invalid_currency, got %v", c, err)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	// Offset required: naive timestamps are rejected.
	if _, err := ParseTimestamp("2026-01-15T10:30:00"); !Is(err, CodeInvalidTimestamp) {
		t.Errorf("naive timestamp should be invalid_timestamp, got %v", err)
	}
	// Offsets are accepted and normalized to UTC.
	got, err := ParseTimestamp("2026-01-15T10:30:00+05:30")
	if err != nil {
		t.Fatalf("+05:30 offset should parse: %v", err)
	}
	if got.Location() != time.UTC || got.Format(time.RFC3339) != "2026-01-15T05:00:00Z" {
		t.Errorf("expected UTC 2026-01-15T05:00:00Z, got %v", got)
	}
}

func TestValidateTimestampRange(t *testing.T) {
	if err := ValidateTimestamp(time.Time{}); !Is(err, CodeInvalidTimestamp) {
		t.Errorf("zero time should be invalid, got %v", err)
	}
	if err := ValidateTimestamp(time.Date(30000, 1, 1, 0, 0, 0, 0, time.UTC)); !Is(err, CodeInvalidTimestamp) {
		t.Errorf("year 30000 should be invalid, got %v", err)
	}
}

func TestValidateLength(t *testing.T) {
	if err := ValidateLength("", 10, CodeInvalidRequest, "X"); !Is(err, CodeInvalidRequest) {
		t.Errorf("empty should be invalid_request, got %v", err)
	}
	long := make([]rune, 11)
	for i := range long {
		long[i] = 'é' // 2-byte rune; length counts characters, not bytes
	}
	if err := ValidateLength(string(long), 10, CodeInvalidRequest, "X"); err == nil {
		t.Error("11 characters should exceed limit of 10")
	}
	if err := ValidateLength(string(long[:10]), 10, CodeInvalidRequest, "X"); err != nil {
		t.Errorf("10 characters should pass: %v", err)
	}
}

func TestIs(t *testing.T) {
	err := NewError(CodeTransactionNotFound, "x")
	if !Is(err, CodeTransactionNotFound) || Is(errors.New("other"), CodeTransactionNotFound) {
		t.Error("Is should match only same-code domain errors")
	}
}
