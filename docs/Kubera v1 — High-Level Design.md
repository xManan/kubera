# Kubera v1 — High-Level Design

## 1. Purpose

Kubera is a standalone, channel-agnostic MCP server for personal-finance transaction tracking. An AI agent acts as the conversational front end: it interprets user messages and transaction notifications, then calls Kubera’s structured MCP tools.
Kubera owns the financial truth. It validates inputs, stores transactions and categories, detects likely duplicates, maintains audit history, and calculates reports. The server must not depend on Telegram, WhatsApp, or any other channel-specific identifier.

## 2. V1 goals

Kubera v1 will provide:

*   Creation, retrieval, listing, correction, and voiding of transactions.
    
*   Money-in and money-out tracking.
    
*   User-managed categories.
    
*   Preservation of original notification text.
    
*   Deterministic, channel-agnostic duplicate detection.
    
*   Daily, monthly, and custom-period summaries.
    
*   Category breakdowns and transaction listings.
    
*   Audit history for transaction updates and voids.
    
*   A stable MCP interface that can be used by different agents and channels.
    

## 3. V1 non-goals

The following are outside the initial design:

*   Multiple users or tenants.
    
*   Bank synchronization.
    
*   Account balances and reconciliation.
    
*   Investment holdings, units, market prices, or portfolio valuation.
    
*   Budgets.
    
*   Recurring transaction rules.
    
*   Permanent deletion of transactions.
    
*   Channel-specific ingestion adapters inside Kubera.
    

## 4. Proposed technology stack

| Area | Technology |
| --- | --- |
| Language | Go |
| Protocol | Model Context Protocol (MCP) |
| MCP implementation | Official or compatible Go MCP SDK |
| Database | SQLite |
| SQLite journaling | Write-Ahead Logging (WAL) |
| Database access | A small repository/data-access layer using a well-supported SQLite driver |
| Schema changes | Versioned SQL migrations |
| Serialization | JSON through MCP tool schemas |
| Testing | Go unit tests, repository integration tests, and MCP contract tests |

The implementation should keep the MCP layer, domain logic, and persistence layer separate so that the database driver or transport can be changed without changing financial rules.

## 5. Architecture overview

```text
AI agent or MCP client
        |
        | MCP requests
        v
MCP transport layer
        |
        v
MCP tool handlers
        |
        v
Application/service layer
  - validation
  - duplicate detection
  - category rules
  - reporting calculations
  - audit generation
        |
        v
Repository layer
        |
        v
SQLite database in WAL mode
```

### 5.1 MCP transport layer

The transport layer accepts MCP requests and returns structured responses. It is responsible for protocol concerns only, including tool registration, request decoding, response encoding, and protocol-level errors.
It must not contain financial calculations or direct SQL queries.

### 5.2 MCP tool handlers

Handlers translate MCP input into application commands and translate application results into stable MCP responses. Each handler should:

*   Validate basic request shape and required fields.
    
*   Call one application-service operation.
    
*   Return deterministic structured output.
    
*   Return actionable errors without exposing SQL or internal implementation details.
    

### 5.3 Application/service layer

The service layer contains Kubera’s business rules, including:

*   Amount and currency validation.
    
*   UTC timestamp validation.
    
*   Category existence and archived-category rules.
    
*   Transaction creation and correction rules.
    
*   Duplicate detection.
    
*   Void behavior.
    
*   Audit-event creation.
    
*   Report date-range and aggregation rules.
    

All writes that modify financial data and audit history should be performed atomically in one database transaction.

### 5.4 Repository layer

The repository layer provides typed operations for transactions, categories, audit events, and reports. It owns SQL statements and database-specific details but not higher-level business decisions.
Reporting queries may use read-only connections or read transactions. Write operations must use transactions where multiple records must remain consistent.

## 6. MCP tool surface

### Transaction tools

*   `create_transaction`
    
*   `get_transaction`
    
*   `list_transactions`
    
*   `update_transaction`
    
*   `void_transaction`
    

### Category tools

*   `list_categories`
    
*   `create_category`
    
*   `update_category`
    
*   `archive_category`
    

### Reporting tools

*   `get_daily_summary`
    
*   `get_monthly_summary`
    
*   `get_transaction_summary`
    
*   `get_category_breakdown`
    

Duplicate detection is part of `create_transaction`. A separate duplicate-check tool may be added later but is not required for v1.

## 7. Core data model

### 7.1 Transactions

A transaction should include:

*   Stable transaction ID.
    
*   Direction: `money_in` or `money_out`.
    
*   Integer amount in minor units.
    
*   Currency code.
    
*   UTC occurrence timestamp.
    
*   Category ID.
    
*   Description or merchant/counterparty.
    
*   Original notification text.
    
*   Optional payment or bank reference ID.
    
*   Creation and update timestamps.
    
*   Optional void timestamp and reason.
    

Amounts must never be stored as floating-point values. For INR, ₹198 is stored as `19800` paise.

### 7.2 Categories

A category should include:

*   Stable category ID.
    
*   User-visible name.
    
*   Optional description of what kinds of transactions belong in the category, exposed to AI clients for classification.
    
*   Archived flag or archive timestamp.
    
*   Creation and update timestamps.
    

Archived categories cannot be assigned to new or updated transactions, but remain associated with historical transactions.

### 7.3 Audit events

An audit event should include:

*   Stable audit-event ID.
    
*   Entity type and entity ID.
    
*   Operation type, such as `created`, `updated`, or `voided`.
    
*   Event timestamp.
    
*   Previous values, where applicable.
    
*   New values, where applicable.
    
*   Optional reason.
    

Audit records should be append-only from the application’s point of view.

## 8. SQLite design

SQLite is appropriate for v1 because Kubera is a single-user, embedded-style service with a modest write workload and a strong requirement for simple, durable local storage.
The database should:

*   Enable WAL mode.
    
*   Enable foreign-key enforcement.
    
*   Use a busy timeout to handle short-lived writer contention.
    
*   Use parameterized SQL for every query.
    
*   Apply schema changes through versioned migrations.
    
*   Use explicit transactions for multi-step writes.
    
*   Create indexes based on the expected transaction-listing and reporting queries.
    

Recommended connection behavior:

*   Limit write concurrency to SQLite’s practical single-writer model.
    
*   Permit concurrent readers where supported by WAL mode.
    
*   Keep write transactions short.
    
*   Avoid holding a write transaction while performing external work.
    
*   Configure a reasonable connection-pool size rather than allowing unbounded connections.
    

The application should verify required SQLite pragmas at startup and fail clearly if the database cannot be opened safely.

## 9. Transaction creation flow

```text
1. MCP client calls create_transaction.
2. Handler decodes the request.
3. Service validates direction, amount, currency, timestamp, and category.
4. Service normalizes relevant duplicate-comparison fields.
5. Repository searches for a likely existing transaction.
6. If a duplicate is found, return a duplicate result and existing transaction.
7. Otherwise begin a database transaction.
8. Insert the transaction.
9. Insert the creation audit event.
10. Commit atomically.
11. Return the canonical transaction and stable ID.
```

The original notification text is retained exactly as supplied. Normalized values used for searching or duplicate detection should be stored separately or derived consistently; they must not replace the original text.

## 10. Duplicate detection

Duplicate detection must not rely on Telegram IDs, WhatsApp IDs, or any other transport identifier.
Matching priority:

1.  Reference ID, when present and sufficiently identifying.
    
2.  Otherwise a deterministic combination of amount, currency, direction, occurrence time, and normalized description/counterparty.
    

The implementation must document its timestamp tolerance and normalization rules. It should avoid identifying legitimate same-amount transactions at different times as duplicates.
A likely duplicate must not silently create another record. The create operation should return a structured result indicating that an existing transaction was found, including the existing transaction ID and the reason or matching fields.
Duplicate detection should be protected against race conditions. The final check and insert must be coordinated so that concurrent identical requests cannot create duplicate records.

## 11. Corrections and voiding

`update_transaction` preserves the stable transaction ID and original notification text. It may update amount, direction, timestamp, description, category, currency, or reference ID.
An update should:

*   Reject archived categories.
    
*   Re-run relevant validation.
    
*   Re-evaluate duplicate risk when identifying fields change.
    
*   Record previous and new values in an audit event.
    
*   Commit the transaction update and audit event atomically.
    

`void_transaction` does not delete data. It records the void timestamp and optional reason, creates an audit event, and excludes the transaction from reports by default.
Repeated void operations should be deterministic and should not create misleading duplicate audit events unless explicitly required by the product rules.

## 12. Reporting design

Kubera performs all totals, grouping, and filtering. The agent must not independently sum raw transactions.
Reports should:

*   Use half-open UTC ranges where possible: `start <= occurred_at < end`.
    
*   Exclude voided transactions by default.
    
*   Return integer minor-unit totals plus currency.
    
*   Return transaction counts and category groupings.
    
*   Use deterministic ordering for categories and largest transactions.
    
*   Validate that the requested range is valid before querying.
    

The monthly report interprets the requested calendar month in UTC. A daily report similarly operates on a UTC date or explicitly supplied UTC range.

## 13. Error handling

Errors should be structured and distinguish at least:

*   Invalid input.
    
*   Missing transaction or category.
    
*   Archived category.
    
*   Duplicate or likely duplicate.
    
*   Already voided transaction.
    
*   Invalid date range.
    
*   Database or internal failure.
    

Error responses should be safe for presentation by an AI agent and should not expose SQL statements, filesystem paths, secrets, or stack traces.

## 14. Security and integrity

*   Validate every MCP input at the server boundary and again where needed in the service layer.
    
*   Use parameterized SQL exclusively.
    
*   Do not trust category IDs, amounts, timestamps, or classifications supplied by the agent.
    
*   Keep secrets and transport credentials outside the database schema.
    
*   Restrict database-file permissions to the service account.
    
*   Avoid logging original notification text unless explicitly configured, because it may contain sensitive financial information.
    
*   Make audit history append-only through application controls.
    
*   Return only the data needed for the requested operation.
    

## 15. Observability

The service should provide structured logs for:

*   Startup and shutdown.
    
*   Database migration status.
    
*   MCP request outcome and duration.
    
*   Tool name and safe request metadata.
    
*   Duplicate detections.
    
*   Validation and database errors.
    

Logs must not include secrets or unnecessarily expose full notification contents. Basic metrics such as request counts, error counts, latency, and database-busy events can be added if the deployment environment supports them.

## 16. Testing strategy

### Unit tests

Test validation, amount handling, UTC rules, category behavior, duplicate normalization, date-range handling, and void/update rules without requiring a live MCP transport.

### Repository integration tests

Run against a temporary SQLite database and verify:

*   Migrations.
    
*   WAL and foreign-key configuration.
    
*   Atomic writes.
    
*   Index-supported queries.
    
*   Audit records.
    
*   Concurrent duplicate creation behavior.
    

### MCP contract tests

Verify that each tool exposes the expected input schema, output shape, error behavior, and stable field names.

### End-to-end tests

Exercise the flow from an MCP request through validation and persistence to the returned canonical result and report output.

## 17. Deployment and operations

Kubera should be packaged as a single Go service with a separately configured SQLite database path. Deployment should support:

*   A dedicated service identity.
    
*   A private database location.
    
*   Configuration through environment variables or a configuration file.
    
*   Graceful shutdown.
    
*   Startup migration checks.
    
*   Regular SQLite backup procedures.
    
*   Restore verification.
    

The database file, WAL file, and shared-memory file must be treated as one database state during operational procedures. Backups should use a SQLite-aware backup mechanism or a safely coordinated copy process rather than copying only the main database file while it is active.

## 18. Future extension points

The design should leave room for:

*   Additional MCP transports.
    
*   Additional ingestion channels.
    
*   Multiple currencies.
    
*   Accounts and transfers.
    
*   Investment holdings and valuation.
    
*   Multi-user authorization.
    
*   More advanced duplicate-review workflows.
    
*   Recurring transactions and budgets.
    

These capabilities should not complicate the v1 domain model unnecessarily. The initial implementation should prioritize correctness, auditability, deterministic behavior, and a small stable MCP surface.

## 19. Key design decisions

1.  Go is used for the service implementation.
    
2.  Kubera uses a Go MCP SDK rather than implementing MCP protocol details from scratch.
    
3.  SQLite is the v1 system of record.
    
4.  SQLite WAL mode supports concurrent reads while retaining a simple single-file deployment model.
    
5.  The application is layered into MCP, service, and repository components.
    
6.  Financial amounts are integer minor units, never floating point.
    
7.  Timestamps are supplied and stored in UTC.
    
8.  Transactions are voided rather than permanently deleted.
    
9.  Audit events are written atomically with data changes.
    
10.  Duplicate detection is channel-agnostic and enforced as part of transaction creation.
     
11.  Reporting calculations are performed by Kubera, not by the calling agent.