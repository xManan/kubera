# Kubera

A standalone, channel-agnostic MCP server for personal-finance transaction tracking. An AI agent acts as the conversational front end; Kubera owns validation, storage, duplicate detection, audit history, and reports.

See `docs/` for the design documents:

- `Kubera v1 — User Requirements and Transaction Flow.md`
- `Kubera v1 — High-Level Design.md`
- `Kubera v1 — Low-Level Design and Implementation.md`

## Stack

- Go + official MCP SDK (`github.com/modelcontextprotocol/go-sdk`), stdio transport
- SQLite (pure-Go driver `modernc.org/sqlite`) with WAL, foreign keys, and embedded migrations
- Layered: `internal/mcp` → `internal/app` (rules) → `internal/repository/sqlite` (SQL)

## Running

```sh
KUBERA_DATABASE_PATH=~/.kubera/kubera.db go run ./cmd/kubera
```

Configuration (environment):

| Variable | Default | Meaning |
| --- | --- | --- |
| `KUBERA_DATABASE_PATH` | required | SQLite database file path |
| `KUBERA_MCP_TRANSPORT` | `stdio` | transport (v1: stdio) |
| `KUBERA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `KUBERA_BUSY_TIMEOUT_MS` | `5000` | SQLite busy timeout |
| `KUBERA_MAX_PAGE_SIZE` | `100` | maximum list page size |
| `KUBERA_MIGRATE_ON_START` | `true` | run pending migrations at startup |

Logs go to stderr as JSON; stdout carries only the MCP protocol.

## MCP tools

Transactions: `create_transaction`, `get_transaction`, `list_transactions`, `update_transaction`, `void_transaction`.
Categories: `list_categories`, `create_category`, `update_category`, `archive_category`. Categories carry an optional description of what belongs in them, which AI clients read when classifying transactions.
Reports: `get_daily_summary`, `get_monthly_summary`, `get_transaction_summary`, `get_category_breakdown`.

Conventions: amounts are positive integer minor units (₹198 → `19800`); timestamps are RFC 3339 UTC with explicit offset; ranges are half-open (`start <= occurred_at < end`); voided transactions are excluded from reports and default listings; updates preserve the transaction ID and original notification text; nothing is ever deleted, only voided.

## Duplicate detection (documented rules)

`create_transaction` returns `status: "duplicate"` instead of writing a second record when a likely match is found:

1. **Reference ID match** (strongest): same `reference_id`, currency, and direction among active transactions.
2. **Field match**: same direction, amount, currency, normalized description, and occurrence time within **±5 minutes** (half-open window).

Normalization for comparison: trim, case-fold, collapse whitespace runs. Stored `description` and `original_notification` keep the caller's text; matching uses the derived `normalized_description` column. Voided transactions never match. The final duplicate check and insert run inside one `BEGIN IMMEDIATE` transaction, so concurrent identical creates cannot both win.

## Operations

- Back up with `sqlite3 kubera.db ".backup backup.db"` (SQLite-aware) rather than copying the live file; treat the `.db`, `-wal`, and `-shm` files as one state.
- Restore: stop the service, replace the database files, start; migrations are idempotent and reopening an already-migrated database is safe.

## Development

```sh
go test ./...      # unit, repository integration, and MCP contract tests
go test -race ./...
go vet ./...
```
