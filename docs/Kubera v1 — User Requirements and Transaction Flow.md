# Kubera v1 — User Requirements and Transaction Flow

## 1. Product purpose

Kubera v1 is a channel-agnostic personal-finance transaction tracker. It receives transaction notifications parsed by an AI agent, stores normalized money-in and money-out transactions, and provides summaries and reports on request.
The initial ingestion channel is Telegram, but Kubera must not depend on Telegram-specific identifiers or behavior. Other channels, such as WhatsApp, may be added later.

## 2. V1 scope

Kubera v1 will:

*   Track money received and money spent.
    
*   Assign transactions to user-defined categories.
    
*   Preserve the original notification text.
    
*   Detect likely duplicate transactions.
    
*   Allow saved transactions to be corrected.
    
*   Allow transactions to be voided.
    
*   Provide daily, monthly, and custom-period reports.
    

The following are out of scope:

*   Accounts and account balances.
    
*   Budgets.
    
*   Bank synchronization.
    
*   Investment portfolio valuation or market prices.
    
*   Recurring transaction rules.
    
*   Reconciliation.
    
*   Multi-user support.
    

## 3. User setup and ingestion flow

The user receives transaction SMS notifications from banks and payment providers. An external automation forwards relevant notifications to a channel, initially a Telegram group. A bot exposes the message to the AI agent.

```text
Bank/payment SMS
    ↓
External forwarding automation
    ↓
Telegram today; other channels later
    ↓
Channel bot exposes message to AI agent
    ↓
Agent identifies and parses transaction
    ↓
Agent determines category
    ↓
Agent calls Kubera MCP
    ↓
Kubera validates, deduplicates, and stores transaction
    ↓
Agent confirms result or asks for clarification
```

Kubera is channel-agnostic. It must not require a Telegram message ID, WhatsApp message ID, or any other channel-specific identifier.

## 4. Agent transaction-processing flow

### 4.1 Identify transaction type

The agent classifies an incoming notification as:

*   `money_in`
    
*   `money_out`
    
*   `not_a_transaction`
    
*   `needs_clarification`
    

Security warnings, promotional messages, failed transactions, and other non-transaction messages should not create records. Failed, reversed, refunded, and pending transactions are ignored in v1 unless the agent/user explicitly identifies them as a completed transaction.

### 4.2 Parse the notification

The agent extracts, where available:

*   Direction: `money_in` or `money_out`.
    
*   Amount.
    
*   Currency.
    
*   Transaction date and time.
    
*   Merchant, sender, recipient, or description.
    
*   Bank/payment reference ID.
    
*   Original notification text.
    
*   Suggested category.
    

The caller is responsible for converting the source date/time to UTC before calling Kubera. Kubera stores transaction date/time in UTC and does not infer or apply a local timezone.

### 4.3 Determine the category

The agent first checks existing Kubera categories and chooses the best match using the notification and the user's prior instructions.
If no suitable category exists, the agent may automatically create a category when the intended category is clear. If the category is ambiguous, the agent must ask the user before creating or assigning one.
The agent should avoid creating multiple categories for minor variations of the same merchant or transaction type.
Examples:

*   `EATCLUB BRANDS PRIVATE` → `Eating Out` or another existing appropriate category.
    
*   Salary credit → `Salary`.
    
*   Mutual-fund contribution → `Investments` or a specific investment category such as `SIP`, depending on the investment.
    

### 4.4 Save through MCP

The agent calls Kubera with the normalized transaction. Kubera validates the request, performs duplicate detection, persists the record, and returns the canonical transaction with its stable ID.

## 5. Transaction model

Each transaction contains:

*   Stable transaction ID.
    
*   Direction: `money_in` or `money_out`.
    
*   Amount stored as an integer minor unit.
    
*   Currency, initially expected to be INR.
    
*   Transaction timestamp in UTC.
    
*   Category.
    
*   Description or merchant/counterparty.
    
*   Original notification text.
    
*   Optional bank/payment reference ID.
    
*   Created timestamp.
    
*   Updated timestamp.
    
*   Optional void timestamp and void reason.
    

Recommended representation:

```json
{
  "id": "txn_01...",
  "direction": "money_out",
  "amount_minor": 19800,
  "currency": "INR",
  "occurred_at": "2026-09-13T14:04:54Z",
  "category_id": "cat_eating_out",
  "description": "EATCLUB BRANDS PRIVATE",
  "reference_id": null,
  "original_text": "Spent Rs.198 On HDFC Bank Card ...",
  "created_at": "...",
  "updated_at": "...",
  "voided_at": null,
  "void_reason": null
}
```

Amounts must not be stored as floating-point values. For INR, `Rs.198` is stored as `19800` paise and `Rs.2100.00` as `210000` paise.

## 6. Investment and SIP transactions

Investment contributions are recorded as `money_out` transactions. They are categorized according to the investment:

*   `Investments` for a general investment contribution.
    
*   `SIP` or a more specific user-created category when appropriate.
    
*   A specific investment category if the user wants to distinguish investments.
    

No account, holdings, units, market value, return, or portfolio tracking is performed in v1.

## 7. Duplicate detection

The same notification may arrive through multiple channels or be forwarded more than once. Kubera must attempt duplicate detection without relying on channel-specific message IDs.
Duplicate matching should use available transaction attributes, especially:

*   Reference ID, when present.
    
*   Amount.
    
*   Transaction timestamp.
    
*   Direction.
    
*   Currency.
    
*   A normalized description or counterparty where useful.
    

Reference ID should be the strongest match when available. When no reference ID exists, Kubera should use a suitable combination of amount, timestamp, direction, and normalized description.
A likely duplicate must not silently create a second transaction. Kubera should return the existing transaction or return a duplicate warning requiring review.
Because notifications may have slightly different formatting or precision, duplicate matching rules should be deterministic and documented. The server should avoid treating two legitimate same-amount transactions at different times as duplicates.

## 8. Categories

Categories are user-managed and can apply to money-in or money-out transactions.
Required capabilities:

*   List categories.
    
*   Create a category.
    
*   Rename or update a category.
    
*   Archive a category.
    
*   Prevent archived categories from being assigned to new transactions.
    
*   Preserve archived categories on historical transactions.
    

Example categories:

*   Salary
    
*   Freelance Income
    
*   Refunds
    
*   Food
    
*   Eating Out
    
*   Groceries
    
*   Transport
    
*   Shopping
    
*   Bills and Utilities
    
*   Rent
    
*   Healthcare
    
*   Education
    
*   Investments
    
*   SIP
    
*   Transfers
    
*   Other
    

## 9. Transaction corrections and voiding

The user must be able to correct a saved transaction when parsing or categorization was incorrect.
Supported updates include:

*   Amount.
    
*   Direction.
    
*   Date/time.
    
*   Description or merchant.
    
*   Category.
    
*   Currency.
    
*   Reference ID.
    

An update preserves the stable transaction ID and original notification text. Every update should create an audit event containing the previous and new values.
Transactions are not permanently deleted in v1. The supported removal operation is `void_transaction`, which marks a transaction as void and optionally records a reason. Voided transactions are excluded from reports by default but remain available for audit and explicit retrieval.

## 10. Reports and summaries

The agent translates natural-language report requests into Kubera reporting-tool calls. Kubera calculates the totals and groupings; the agent must not independently sum raw transactions.
Required reports:

### Daily summary

For a UTC date or explicitly specified UTC range:

*   Total money in.
    
*   Total money out.
    
*   Net amount.
    
*   Transaction count.
    
*   Money-out breakdown by category.
    
*   Money-in breakdown by category.
    

### Monthly summary

For a specified calendar month in UTC:

*   Total money in.
    
*   Total money out.
    
*   Net amount.
    
*   Transaction count.
    
*   Spending by category.
    
*   Income by category.
    
*   Largest transactions.
    

### Custom-period summary

For a specified UTC start and end timestamp/date:

*   Total money in.
    
*   Total money out.
    
*   Net amount.
    
*   Category breakdown.
    
*   Transaction count.
    

### Transaction listing

Support filters for:

*   UTC date/time range.
    
*   Direction.
    
*   Category.
    
*   Minimum or maximum amount.
    
*   Merchant/description text.
    
*   Reference ID.
    
*   Include or exclude voided transactions.
    

## 11. Initial MCP tool surface

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
    

Duplicate detection should be part of `create_transaction`; a separate duplicate-check tool is optional and not required for the basic flow.

## 12. Responsibilities

### AI agent

*   Read the channel message.
    
*   Determine whether it represents a completed transaction.
    
*   Extract and normalize transaction fields.
    
*   Convert the source date/time to UTC.
    
*   Infer a category.
    
*   Automatically create a category when the intended category is clear.
    
*   Ask the user when category selection is ambiguous.
    
*   Call Kubera MCP tools.
    
*   Ask for clarification when required fields or transaction meaning are unclear.
    
*   Present confirmations and reports in a human-friendly form.
    

### Kubera MCP server

*   Validate all inputs.
    
*   Enforce valid directions and required fields.
    
*   Store precise integer amounts.
    
*   Store timestamps in UTC.
    
*   Store transactions and categories.
    
*   Perform channel-agnostic duplicate detection.
    
*   Calculate summaries and reports.
    
*   Preserve original notification text.
    
*   Maintain audit history for updates and voids.
    
*   Return structured, deterministic results.
    

## 13. Confirmed product decisions

1.  Investment contributions are `money_out` transactions.
    
2.  Investment categories may be general (`Investments`) or specific (`SIP` or another user-created category).
    
3.  The agent may create a category automatically when the category is clear; it must ask when ambiguous.
    
4.  Failed, reversed, refunded, and pending transactions are ignored in v1.
    
5.  Transactions are voided rather than permanently deleted.
    
6.  All transaction date/time values are stored in UTC; the caller performs local-time conversion.
    
7.  Duplicate detection is channel-agnostic and uses reference ID, amount, timestamp, and related transaction attributes rather than Telegram or WhatsApp message IDs.