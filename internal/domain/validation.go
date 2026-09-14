package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Documented maximum lengths (UTF-8 characters) to prevent resource
// exhaustion. See LLD section 8.1.
const (
	MaxDescriptionLength         = 500
	MaxNotificationLength        = 10000
	MaxReferenceLength           = 256
	MaxCategoryNameLength        = 100
	MaxCategoryDescriptionLength = 500
	MaxVoidReasonLength          = 1000
)

// ValidateDirection checks that d is a known direction.
func ValidateDirection(d Direction) error {
	switch d {
	case MoneyIn, MoneyOut:
		return nil
	default:
		return NewError(CodeInvalidDirection, "Direction must be \"money_in\" or \"money_out\".")
	}
}

// ValidateAmount checks that amount is a positive minor-unit integer.
func ValidateAmount(amount int64) error {
	if amount <= 0 {
		return NewError(CodeInvalidAmount, "Amount must be a positive integer in minor units (e.g. paise).")
	}
	return nil
}

// ValidateCurrency checks that c is an uppercase ISO 4217-style three-letter code.
func ValidateCurrency(c CurrencyCode) error {
	if len(c) != 3 {
		return NewError(CodeInvalidCurrency, "Currency must be a three-letter ISO 4217 code.")
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return NewError(CodeInvalidCurrency, "Currency must be a three-letter uppercase ISO 4217 code.")
		}
	}
	return nil
}

// ParseTimestamp parses an RFC 3339 timestamp that must carry an explicit
// timezone offset, returning the UTC instant.
func ParseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, NewError(CodeInvalidTimestamp,
			"Timestamp must be a valid RFC 3339 value with an explicit timezone offset, e.g. 2026-01-15T10:30:00Z.")
	}
	return t.UTC(), nil
}

// ValidateTimestamp rejects zero or out-of-range instants.
func ValidateTimestamp(t time.Time) error {
	if t.IsZero() || t.Year() < 1 || t.Year() > 9999 {
		return NewError(CodeInvalidTimestamp, "Timestamp is outside the supported range.")
	}
	return nil
}

// ValidateMaxLength checks that s is within max UTF-8 characters; empty is allowed.
func ValidateMaxLength(s string, max int, code ErrorCode, label string) error {
	if utf8.RuneCountInString(s) > max {
		return NewError(code, fmt.Sprintf("%s exceeds the maximum length of %d characters.", label, max))
	}
	return nil
}

// ValidateLength checks that s is a non-empty string within max UTF-8 characters.
func ValidateLength(s string, max int, code ErrorCode, label string) error {
	if s == "" {
		return NewError(code, label+" must not be empty.")
	}
	if utf8.RuneCountInString(s) > max {
		return NewError(code, fmt.Sprintf("%s exceeds the maximum length of %d characters.", label, max))
	}
	return nil
}
