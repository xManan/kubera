package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
)

// CreateTransactionInput is the validated-at-boundary create request.
type CreateTransactionInput struct {
	Direction            domain.Direction
	AmountMinor          int64
	Currency             domain.CurrencyCode
	OccurredAt           time.Time
	CategoryID           domain.CategoryID
	Description          string
	OriginalNotification string
	ReferenceID          string
}

// DuplicateInfo describes an existing transaction that blocked creation.
type DuplicateResult struct {
	Existing domain.Transaction `json:"existing_transaction"`
	Reason   string             `json:"match_reason"`
}

// CreateTransactionResult makes the create-vs-duplicate outcome explicit.
type CreateTransactionResult struct {
	Status      string             `json:"status"` // "created" or "duplicate"
	Transaction domain.Transaction `json:"transaction,omitempty"`
	Duplicate   *DuplicateResult   `json:"duplicate,omitempty"`
}

// CreateTransaction validates, deduplicates, and persists a transaction plus
// its creation audit event atomically.
func (s *Services) CreateTransaction(ctx context.Context, in CreateTransactionInput) (CreateTransactionResult, error) {
	var out CreateTransactionResult

	in.Description = trim(in.Description)
	in.ReferenceID = duplicate.NormalizeReference(in.ReferenceID)
	if err := validateTransactionInput(in); err != nil {
		return out, err
	}

	now := s.now()
	t := domain.Transaction{
		Direction: in.Direction, AmountMinor: in.AmountMinor, Currency: in.Currency,
		OccurredAt: in.OccurredAt.UTC(), CategoryID: in.CategoryID,
		Description: in.Description, OriginalNotification: in.OriginalNotification,
		ReferenceID: in.ReferenceID,
		CreatedAt:   now, UpdatedAt: now,
	}
	candidate := toCandidate(t)

	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		cat, err := s.Categories.Get(ctx, tx, in.CategoryID)
		if err != nil {
			return err
		}
		if cat.IsArchived() {
			return domain.NewError(domain.CodeArchivedCategory,
				"The category is archived and cannot be assigned to transactions.")
		}

		existing, isDup, err := s.Transactions.FindLikelyDuplicate(ctx, tx, candidate, duplicate.Tolerance)
		if err != nil {
			return err
		}
		if isDup {
			out = CreateTransactionResult{
				Status: "duplicate",
				Duplicate: &DuplicateResult{
					Existing: existing,
					Reason:   duplicateReason(candidate, existing),
				},
			}
			return nil
		}

		t.ID = domain.TransactionID(s.id("txn"))
		if err := s.Transactions.Insert(ctx, tx, t); err != nil {
			return err
		}
		if err := s.Audits.Append(ctx, tx, domain.AuditEvent{
			ID: domain.AuditEventID(s.id("aud")), EntityType: domain.EntityTypeTransaction,
			EntityID: string(t.ID), Operation: domain.OpCreated, OccurredAt: now,
			Current: auditSnapshot(t),
		}); err != nil {
			return err
		}
		out = CreateTransactionResult{Status: "created", Transaction: t}
		return nil
	})
	if err != nil {
		return CreateTransactionResult{}, err
	}
	return out, nil
}

// GetTransaction returns the canonical transaction, including void fields.
func (s *Services) GetTransaction(ctx context.Context, id domain.TransactionID) (domain.Transaction, error) {
	return s.Transactions.Get(ctx, s.DB, id)
}

// ListTransactions returns a deterministic page of transactions.
func (s *Services) ListTransactions(ctx context.Context, f domain.TransactionFilter) ([]domain.Transaction, domain.Page, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Start != nil && f.End != nil && !f.End.After(*f.Start) {
		return nil, domain.Page{}, domain.NewError(domain.CodeInvalidDateRange,
			"The end of the date range must be after its start.")
	}
	return s.Transactions.List(ctx, s.DB, f)
}

// UpdateTransactionInput is a patch over mutable transaction fields. Nil
// fields are unchanged; ReferenceID of "" clears the reference. The original
// notification text is immutable and the stable ID is preserved.
type UpdateTransactionInput struct {
	ID          domain.TransactionID
	Direction   *domain.Direction
	AmountMinor *int64
	Currency    *domain.CurrencyCode
	OccurredAt  *time.Time
	CategoryID  *domain.CategoryID
	Description *string
	ReferenceID *string
}

// UpdateTransaction applies a non-empty patch atomically with one `updated`
// audit event, rejecting updates to voided transactions and archived
// categories, and re-running duplicate detection when identifying fields change.
func (s *Services) UpdateTransaction(ctx context.Context, in UpdateTransactionInput) (domain.Transaction, error) {
	if isPatchEmpty(in) {
		return domain.Transaction{}, domain.NewError(domain.CodeEmptyUpdate,
			"Provide at least one field to update.")
	}

	var updated domain.Transaction
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.Transactions.Get(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if cur.IsVoided() {
			return domain.NewError(domain.CodeTransactionAlreadyVoided,
				"The transaction is already voided and cannot be updated.").
				With("transaction_id", string(cur.ID))
		}

		t := cur
		if in.Direction != nil {
			if err := domain.ValidateDirection(*in.Direction); err != nil {
				return err
			}
			t.Direction = *in.Direction
		}
		if in.AmountMinor != nil {
			if err := domain.ValidateAmount(*in.AmountMinor); err != nil {
				return err
			}
			t.AmountMinor = *in.AmountMinor
		}
		if in.Currency != nil {
			if err := domain.ValidateCurrency(*in.Currency); err != nil {
				return err
			}
			t.Currency = *in.Currency
		}
		if in.OccurredAt != nil {
			if err := domain.ValidateTimestamp(in.OccurredAt.UTC()); err != nil {
				return err
			}
			t.OccurredAt = in.OccurredAt.UTC()
		}
		if in.Description != nil {
			d := trim(*in.Description)
			if err := domain.ValidateLength(d, domain.MaxDescriptionLength,
				domain.CodeInvalidRequest, "Description"); err != nil {
				return err
			}
			t.Description = d
		}
		if in.ReferenceID != nil {
			t.ReferenceID = duplicate.NormalizeReference(*in.ReferenceID)
		}
		if in.CategoryID != nil {
			t.CategoryID = *in.CategoryID
		}
		cat, err := s.Categories.Get(ctx, tx, t.CategoryID)
		if err != nil {
			return err
		}
		if cat.IsArchived() {
			return domain.NewError(domain.CodeArchivedCategory,
				"The category is archived and cannot be assigned to transactions.")
		}

		// Re-check duplicates only when identifying fields changed, excluding self.
		if identifyingChanged(cur, t) {
			existing, isDup, err := s.Transactions.FindLikelyDuplicate(ctx, tx, toCandidate(t), duplicate.Tolerance)
			if err != nil {
				return err
			}
			if isDup && existing.ID != cur.ID {
				return domain.NewError(domain.CodeDuplicateTransaction,
					"The update would make this transaction duplicate an existing one.").
					With("existing_transaction_id", string(existing.ID))
			}
		}

		now := s.now()
		t.UpdatedAt = now
		if err := s.Transactions.Update(ctx, tx, t); err != nil {
			return err
		}
		if err := s.Audits.Append(ctx, tx, domain.AuditEvent{
			ID: domain.AuditEventID(s.id("aud")), EntityType: domain.EntityTypeTransaction,
			EntityID: string(t.ID), Operation: domain.OpUpdated, OccurredAt: now,
			Previous: auditSnapshot(cur), Current: auditSnapshot(t),
		}); err != nil {
			return err
		}
		updated = t
		return nil
	})
	if err != nil {
		return domain.Transaction{}, err
	}
	return updated, nil
}

// VoidTransaction marks a transaction voided (never deleted) atomically with
// a `voided` audit event. Repeated voids are deterministic errors.
func (s *Services) VoidTransaction(ctx context.Context, id domain.TransactionID, reason string) (domain.Transaction, error) {
	reason = trim(reason)
	if reason != "" {
		if err := domain.ValidateLength(reason, domain.MaxVoidReasonLength,
			domain.CodeInvalidRequest, "Void reason"); err != nil {
			return domain.Transaction{}, err
		}
	}

	var voided domain.Transaction
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.Transactions.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.IsVoided() {
			return domain.NewError(domain.CodeTransactionAlreadyVoided,
				"The transaction is already voided.").With("transaction", txSnapshotJSON(cur))
		}
		now := s.now()
		cur.VoidedAt = &now
		cur.VoidReason = reason
		cur.UpdatedAt = now
		if err := s.Transactions.Void(ctx, tx, id, now, reason); err != nil {
			return err
		}
		if err := s.Audits.Append(ctx, tx, domain.AuditEvent{
			ID: domain.AuditEventID(s.id("aud")), EntityType: domain.EntityTypeTransaction,
			EntityID: string(id), Operation: domain.OpVoided, OccurredAt: now,
			Previous: auditSnapshot(stripVoid(cur)), Current: auditSnapshot(cur),
			Reason: reason,
		}); err != nil {
			return err
		}
		voided = cur
		return nil
	})
	if err != nil {
		return domain.Transaction{}, err
	}
	return voided, nil
}

// ListAuditEvents returns the append-only audit history for a transaction.
func (s *Services) ListAuditEvents(ctx context.Context, id domain.TransactionID) ([]domain.AuditEvent, error) {
	if _, err := s.GetTransaction(ctx, id); err != nil {
		return nil, err
	}
	return s.Audits.ListForEntity(ctx, s.DB, domain.EntityTypeTransaction, string(id))
}

// --- helpers ---

func trim(s string) string {
	return strings.TrimSpace(s)
}

func validateTransactionInput(in CreateTransactionInput) error {
	if err := domain.ValidateDirection(in.Direction); err != nil {
		return err
	}
	if err := domain.ValidateAmount(in.AmountMinor); err != nil {
		return err
	}
	if err := domain.ValidateCurrency(in.Currency); err != nil {
		return err
	}
	if err := domain.ValidateTimestamp(in.OccurredAt); err != nil {
		return err
	}
	if in.CategoryID == "" {
		return domain.NewError(domain.CodeInvalidRequest, "Category ID is required.")
	}
	if in.Description != "" {
		if err := domain.ValidateLength(in.Description, domain.MaxDescriptionLength,
			domain.CodeInvalidRequest, "Description"); err != nil {
			return err
		}
	}
	if in.OriginalNotification != "" {
		if err := domain.ValidateLength(in.OriginalNotification, domain.MaxNotificationLength,
			domain.CodeInvalidRequest, "Original notification"); err != nil {
			return err
		}
	}
	if in.ReferenceID != "" {
		if err := domain.ValidateLength(in.ReferenceID, domain.MaxReferenceLength,
			domain.CodeInvalidRequest, "Reference ID"); err != nil {
			return err
		}
	}
	return nil
}

// identifyingChanged reports whether duplicate-identifying fields differ.
func identifyingChanged(a, b domain.Transaction) bool {
	return a.Direction != b.Direction || a.AmountMinor != b.AmountMinor ||
		a.Currency != b.Currency || !a.OccurredAt.Equal(b.OccurredAt) ||
		a.Description != b.Description || a.ReferenceID != b.ReferenceID
}

// duplicateReason documents which matching priority fired.
func duplicateReason(c duplicate.Candidate, existing domain.Transaction) string {
	if c.ReferenceID != "" && existing.ReferenceID == c.ReferenceID {
		return duplicate.MatchReasonReference
	}
	return duplicate.MatchReasonFields
}

// stripVoid is used to record the pre-void state in the void audit event.
func stripVoid(t domain.Transaction) domain.Transaction {
	t.VoidedAt = nil
	t.VoidReason = ""
	return t
}

// txSnapshotJSON embeds a transaction in error details safely.
func txSnapshotJSON(t domain.Transaction) map[string]any {
	b, _ := json.Marshal(map[string]any{
		"id":           string(t.ID),
		"direction":    string(t.Direction),
		"amount_minor": t.AmountMinor,
		"currency":     string(t.Currency),
		"occurred_at":  rfc3339(t.OccurredAt),
		"voided_at":    voidString(t.VoidedAt),
		"void_reason":  t.VoidReason,
	})
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{"transaction_id": string(t.ID)}
	}
	return m
}

func isPatchEmpty(in UpdateTransactionInput) bool {
	return in.Direction == nil && in.AmountMinor == nil && in.Currency == nil &&
		in.OccurredAt == nil && in.CategoryID == nil && in.Description == nil &&
		in.ReferenceID == nil
}
