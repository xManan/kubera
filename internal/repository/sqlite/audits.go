package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"kubera/internal/domain"
	"kubera/internal/repository"
)

type auditRepo struct{}

// NewAuditRepository returns the SQLite-backed append-only audit repository.
func NewAuditRepository() repository.AuditRepository { return auditRepo{} }

func (auditRepo) Append(ctx context.Context, tx *sql.Tx, e domain.AuditEvent) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO audit_events (id, entity_type, entity_id, operation, occurred_at, previous_json, current_json, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		string(e.ID), e.EntityType, e.EntityID, e.Operation, timeString(e.OccurredAt),
		jsonOrNull(e.Previous), jsonOrNull(e.Current), nullString(e.Reason))
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

func jsonOrNull(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func (auditRepo) ListForEntity(ctx context.Context, q repository.Queryer, entityType, entityID string) ([]domain.AuditEvent, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, entity_type, entity_id, operation, occurred_at, previous_json, current_json, reason FROM audit_events WHERE entity_type = ? AND entity_id = ? ORDER BY occurred_at ASC, id ASC",
		entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		var (
			e          domain.AuditEvent
			occurredAt string
			previous   sql.NullString
			current    sql.NullString
			reason     sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.Operation, &occurredAt, &previous, &current, &reason); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		if e.OccurredAt, err = parseTime(occurredAt); err != nil {
			return nil, fmt.Errorf("scan audit event occurred_at: %w", err)
		}
		if previous.Valid {
			e.Previous = []byte(previous.String)
		}
		if current.Valid {
			e.Current = []byte(current.String)
		}
		e.Reason = reason.String
		out = append(out, e)
	}
	return out, rows.Err()
}
