package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"kubera/internal/domain"
)

// migrationsFS embeds the versioned SQL migrations in the binary.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// driverName for modernc.org/sqlite (pure Go, no cgo).
const driverName = "sqlite"

// Open opens the SQLite database with WAL, foreign keys, busy timeout, and
// immediate write transactions (so duplicate check + insert serialize), then
// verifies the pragmas actually took effect.
func Open(path string, busyTimeoutMS int) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_txlock=immediate&_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)",
		path, busyTimeoutMS,
	)
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	// ponytail: single shared pool of 4; split read/write pools if report
	// latency ever contends with writes.

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, domain.NewError(domain.CodeDatabaseUnavailable, "The database could not be opened.").With("cause", err.Error())
	}

	var mode, fk string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || !strings.EqualFold(mode, "wal") {
		db.Close()
		return nil, fmt.Errorf("sqlite WAL mode not active (got %q): %w", mode, err)
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != "1" {
		db.Close()
		return nil, fmt.Errorf("sqlite foreign_keys not enabled (got %q): %w", fk, err)
	}
	return db, nil
}

// Migrate applies pending embedded migrations in order, one transaction each,
// and returns the resulting schema version.
func Migrate(ctx context.Context, db *sql.DB) (int, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return 0, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var current int
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}

	for _, name := range names {
		var version int
		if _, err := fmt.Sscanf(name, "%04d", &version); err != nil {
			return current, fmt.Errorf("migration file %q: bad version prefix: %w", name, err)
		}
		if version <= current {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return current, fmt.Errorf("read migration %q: %w", name, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return current, fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return current, fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
			version, nowUTC().Format(time.RFC3339)); err != nil {
			tx.Rollback()
			return current, fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return current, fmt.Errorf("commit migration %d: %w", version, err)
		}
		current = version
	}
	return current, nil
}

func nowUTC() time.Time { return time.Now().UTC() }

// timeString canonicalizes timestamps for storage: UTC, second precision, RFC
// 3339 with Z. Second precision keeps lexicographic string order equal to
// chronological order, which the reporting queries rely on.
func timeString(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// nullString maps the empty string to SQL NULL (reference_id, etc.).
func nullString(s string) driver.Value {
	if s == "" {
		return nil
	}
	return s
}

func strPtr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}
