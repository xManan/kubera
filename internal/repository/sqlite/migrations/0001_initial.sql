-- Kubera v1 initial schema. Timestamps are canonical UTC RFC 3339 strings.
-- (schema_migrations is created and owned by the migration runner.)

CREATE TABLE categories (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    name_normalized TEXT NOT NULL,
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (length(trim(name)) > 0)
);

CREATE UNIQUE INDEX categories_active_name_unique
    ON categories(name_normalized)
    WHERE archived_at IS NULL;

CREATE TABLE transactions (
    id TEXT PRIMARY KEY NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('money_in', 'money_out')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    category_id TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    original_notification TEXT NOT NULL DEFAULT '',
    reference_id TEXT,
    normalized_description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    voided_at TEXT,
    void_reason TEXT,
    FOREIGN KEY (category_id) REFERENCES categories(id),
    CHECK (length(currency) = 3),
    CHECK ((voided_at IS NULL AND void_reason IS NULL) OR voided_at IS NOT NULL)
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    previous_json TEXT,
    current_json TEXT,
    reason TEXT
);

CREATE INDEX transactions_occurred_at_idx
    ON transactions(occurred_at);

CREATE INDEX transactions_category_occurred_idx
    ON transactions(category_id, occurred_at);

CREATE INDEX transactions_reference_idx
    ON transactions(reference_id)
    WHERE reference_id IS NOT NULL;

CREATE INDEX transactions_active_occurred_idx
    ON transactions(voided_at, occurred_at);

CREATE INDEX transactions_dup_idx
    ON transactions(voided_at, direction, currency, amount_minor, occurred_at);

CREATE INDEX audit_entity_idx
    ON audit_events(entity_type, entity_id, occurred_at);
