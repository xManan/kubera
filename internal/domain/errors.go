package domain

import (
	"fmt"
)

// ErrorCode is a stable application error code safe to expose to MCP clients.
type ErrorCode string

const (
	CodeInvalidRequest           ErrorCode = "invalid_request"
	CodeInvalidDirection         ErrorCode = "invalid_direction"
	CodeInvalidAmount            ErrorCode = "invalid_amount"
	CodeInvalidCurrency          ErrorCode = "invalid_currency"
	CodeInvalidTimestamp         ErrorCode = "invalid_timestamp"
	CodeInvalidDateRange         ErrorCode = "invalid_date_range"
	CodeTransactionNotFound      ErrorCode = "transaction_not_found"
	CodeCategoryNotFound         ErrorCode = "category_not_found"
	CodeCategoryAlreadyExists    ErrorCode = "category_already_exists"
	CodeArchivedCategory         ErrorCode = "archived_category"
	CodeTransactionAlreadyVoided ErrorCode = "transaction_already_voided"
	CodeDuplicateTransaction     ErrorCode = "duplicate_transaction"
	CodeEmptyUpdate              ErrorCode = "empty_update"
	CodeConflict                 ErrorCode = "conflict"
	CodeDatabaseUnavailable      ErrorCode = "database_unavailable"
	CodeMigrationFailed          ErrorCode = "migration_failed"
	CodeInternal                 ErrorCode = "internal_error"
)

// Error is a typed application error with a stable code.
type Error struct {
	Code    ErrorCode
	Message string
	Details map[string]any
}

func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// With attaches structured details to the error.
func (e *Error) With(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
	return e
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// safeMessage returns a client-safe message, substituting internal codes for
// internal/database errors.
func (e *Error) SafeMessage() string {
	switch e.Code {
	case CodeInternal, CodeDatabaseUnavailable, CodeMigrationFailed:
		return "An internal error occurred. The operation could not be completed."
	default:
		return e.Message
	}
}

// Is reports whether err is a domain error with the given code.
func Is(err error, code ErrorCode) bool {
	de, ok := err.(*Error)
	return ok && de.Code == code
}
