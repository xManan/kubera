package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/app"
	"kubera/internal/repository/sqlite"
)

type fixture struct {
	ctx        context.Context
	session    *mcp.ClientSession
	svc        *app.Services
	nowCounter int
}

// fixedNow hands out deterministic ascending timestamps.
func (f *fixture) now() time.Time {
	f.nowCounter++
	return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC).Add(time.Duration(f.nowCounter) * time.Second)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := sqlite.Open(t.TempDir()+"/test.db", 5000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	f := &fixture{ctx: context.Background(), svc: &app.Services{
		DB:           db,
		Transactions: sqlite.NewTransactionRepository(),
		Categories:   sqlite.NewCategoryRepository(),
		Audits:       sqlite.NewAuditRepository(),
		Reports:      sqlite.NewReportRepository(),
	}}
	f.svc.Now = f.now
	f.svc.NewID = func(prefix string) string {
		f.nowCounter++
		return prefix + "_" + string(rune('a'+f.nowCounter%26)) + "0000"
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := New(f.svc, log, "test")
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	serverT, clientT := mcp.NewInMemoryTransports()
	if _, err := server.Connect(f.ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(f.ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	f.session = session
	return f
}

// call invokes a tool and unmarshals the structured result into out.
func (f *fixture) call(t *testing.T, name string, args string) (map[string]any, bool) {
	t.Helper()
	res, err := f.session.CallTool(f.ctx, &mcp.CallToolParams{Name: name, Arguments: json.RawMessage(args)})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var out map[string]any
	if res.StructuredContent == nil {
		t.Fatalf("%s: empty structured content", name)
	}
	b, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s: unmarshal result: %v", name, err)
	}
	return out, res.IsError
}

func mustCreated(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	if out["status"] != "created" {
		t.Fatalf("expected status created, got %v", out["status"])
	}
	tx, ok := out["transaction"].(map[string]any)
	if !ok {
		t.Fatal("missing transaction object")
	}
	return tx
}

func TestToolSurface(t *testing.T) {
	f := newFixture(t)
	tools, err := f.session.ListTools(f.ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"create_transaction": false, "get_transaction": false, "list_transactions": false,
		"update_transaction": false, "void_transaction": false,
		"list_categories": false, "create_category": false, "update_category": false,
		"archive_category":  false,
		"get_daily_summary": false, "get_monthly_summary": false,
		"get_transaction_summary": false, "get_category_breakdown": false,
	}
	for _, tool := range tools.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		want[tool.Name] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestEndToEndFlow(t *testing.T) {
	f := newFixture(t)

	// Category setup.
	out, isErr := f.call(t, "create_category", `{"name": "Eating Out"}`)
	if isErr {
		t.Fatalf("create_category failed: %v", out)
	}
	cat := out["category"].(map[string]any)
	catID := cat["id"].(string)

	// Duplicate name (case-insensitive) is rejected.
	out, isErr = f.call(t, "create_category", `{"name": "eating out"}`)
	if !isErr || out["error"].(map[string]any)["code"] != "category_already_exists" {
		t.Fatalf("expected category_already_exists, got %v isErr=%v", out, isErr)
	}

	// Create a transaction.
	out, isErr = f.call(t, "create_transaction", `{
		"direction": "money_out", "amount_minor": 19800, "currency": "INR",
		"occurred_at": "2026-09-13T14:04:54Z", "category_id": "`+catID+`",
		"description": "EATCLUB BRANDS PRIVATE",
		"original_notification": "Spent Rs.198 On HDFC Bank Card"
	}`)
	if isErr {
		t.Fatalf("create_transaction failed: %v", out)
	}
	tx := mustCreated(t, out)
	txID := tx["id"].(string)
	if tx["original_text"] != "Spent Rs.198 On HDFC Bank Card" {
		t.Fatalf("original text not preserved: %v", tx["original_text"])
	}
	if tx["amount_minor"].(float64) != 19800 {
		t.Fatalf("amount must be integer minor units: %v", tx["amount_minor"])
	}

	// Identical retry → structured duplicate result, no second record.
	out, isErr = f.call(t, "create_transaction", `{
		"direction": "money_out", "amount_minor": 19800, "currency": "INR",
		"occurred_at": "2026-09-13T14:07:54Z", "category_id": "`+catID+`",
		"description": "eatclub brands private"
	}`)
	if isErr {
		t.Fatalf("duplicate retry errored: %v", out)
	}
	if out["status"] != "duplicate" {
		t.Fatalf("expected duplicate status, got %v", out["status"])
	}
	if out["existing_transaction"].(map[string]any)["id"] != txID {
		t.Fatalf("duplicate should point at existing transaction, got %v", out["existing_transaction"])
	}
	if out["match_reason"] != "amount_currency_direction_description_time" {
		t.Fatalf("unexpected match_reason: %v", out["match_reason"])
	}

	// Update preserves ID and original text.
	out, isErr = f.call(t, "update_transaction", `{
		"transaction_id": "`+txID+`", "amount_minor": 19900, "description": "EATCLUB"
	}`)
	if isErr {
		t.Fatalf("update failed: %v", out)
	}
	updated := out["transaction"].(map[string]any)
	if updated["id"] != txID {
		t.Fatal("update must preserve the stable ID")
	}
	if updated["original_text"] != "Spent Rs.198 On HDFC Bank Card" {
		t.Fatal("original notification text must be immutable through update")
	}
	if updated["amount_minor"].(float64) != 19900 {
		t.Fatal("update should change amount")
	}

	// Void.
	out, isErr = f.call(t, "void_transaction", `{"transaction_id": "`+txID+`", "reason": "wrong entry"}`)
	if isErr {
		t.Fatalf("void failed: %v", out)
	}
	if out["transaction"].(map[string]any)["voided_at"] == nil {
		t.Fatal("voided transaction must carry voided_at")
	}

	// Second void is a deterministic typed error.
	out, isErr = f.call(t, "void_transaction", `{"transaction_id": "`+txID+`"}`)
	if !isErr || out["error"].(map[string]any)["code"] != "transaction_already_voided" {
		t.Fatalf("expected transaction_already_voided, got %v isErr=%v", out, isErr)
	}

	// Reports exclude voided transactions.
	out, isErr = f.call(t, "get_daily_summary", `{"date": "2026-09-13"}`)
	if isErr {
		t.Fatalf("daily summary failed: %v", out)
	}
	totals := out["currency_totals"].([]any)
	if len(totals) != 0 {
		t.Fatalf("voided transaction must be excluded from reports, got %v", totals)
	}

	// A non-voided transaction shows up.
	out, isErr = f.call(t, "create_transaction", `{
		"direction": "money_in", "amount_minor": 500000, "currency": "INR",
		"occurred_at": "2026-09-13T10:00:00Z", "category_id": "`+catID+`",
		"description": "Refund"
	}`)
	if isErr {
		t.Fatalf("second create failed: %v", out)
	}
	out, isErr = f.call(t, "get_daily_summary", `{"date": "2026-09-13"}`)
	if isErr {
		t.Fatalf("daily summary failed: %v", out)
	}
	totals = out["currency_totals"].([]any)
	if len(totals) != 1 {
		t.Fatalf("expected one currency total, got %v", totals)
	}
	total := totals[0].(map[string]any)
	if total["money_in_minor"].(float64) != 500000 || total["money_out_minor"].(float64) != 0 {
		t.Fatalf("unexpected totals: %v", total)
	}
	if total["net_minor"].(float64) != 500000 {
		t.Fatalf("net = in - out, got %v", total["net_minor"])
	}
}

func TestErrorMapping(t *testing.T) {
	f := newFixture(t)

	out, isErr := f.call(t, "get_transaction", `{"transaction_id": "txn_nope"}`)
	if !isErr || out["error"].(map[string]any)["code"] != "transaction_not_found" {
		t.Fatalf("expected transaction_not_found, got %v isErr=%v", out, isErr)
	}

	out, isErr = f.call(t, "create_transaction", `{
		"direction": "sideways", "amount_minor": 100, "currency": "INR",
		"occurred_at": "2026-09-13T10:00:00Z", "category_id": "cat_x"
	}`)
	if !isErr || out["error"].(map[string]any)["code"] != "invalid_direction" {
		t.Fatalf("expected invalid_direction, got %v isErr=%v", out, isErr)
	}

	// Naive timestamp (no offset) is rejected.
	out, isErr = f.call(t, "create_transaction", `{
		"direction": "money_out", "amount_minor": 100, "currency": "INR",
		"occurred_at": "2026-09-13T10:00:00", "category_id": "cat_x"
	}`)
	if !isErr || out["error"].(map[string]any)["code"] != "invalid_timestamp" {
		t.Fatalf("expected invalid_timestamp, got %v isErr=%v", out, isErr)
	}
}

func TestCategoryDescriptions(t *testing.T) {
	f := newFixture(t)

	// Create with a description; listing exposes it for classification.
	out, isErr := f.call(t, "create_category",
		`{"name": "Eating Out", "description": "Restaurants, food delivery, coffee shops."}`)
	if isErr {
		t.Fatalf("create_category failed: %v", out)
	}
	cat := out["category"].(map[string]any)
	if cat["description"] != "Restaurants, food delivery, coffee shops." {
		t.Fatalf("description not stored: %v", cat["description"])
	}
	catID := cat["id"].(string)

	// Description is optional.
	out, _ = f.call(t, "create_category", `{"name": "No Desc"}`)
	if out["category"].(map[string]any)["description"] != "" {
		t.Fatalf("description should default to empty: %v", out["category"])
	}

	out, _ = f.call(t, "list_categories", `{}`)
	var found bool
	for _, c := range out["categories"].([]any) {
		if c.(map[string]any)["id"] == catID {
			found = true
			if c.(map[string]any)["description"] != "Restaurants, food delivery, coffee shops." {
				t.Fatalf("list_categories must expose description: %v", c)
			}
		}
	}
	if !found {
		t.Fatal("created category missing from listing")
	}

	// Description-only update leaves the name alone.
	out, isErr = f.call(t, "update_category",
		`{"category_id": "`+catID+`", "description": "Food delivery and dining."}`)
	if isErr {
		t.Fatalf("description-only update failed: %v", out)
	}
	cat = out["category"].(map[string]any)
	if cat["name"] != "Eating Out" || cat["description"] != "Food delivery and dining." {
		t.Fatalf("unexpected patch result: %v", cat)
	}

	// Empty string clears the description; omitting keeps it.
	out, isErr = f.call(t, "update_category", `{"category_id": "`+catID+`", "description": ""}`)
	if isErr || out["category"].(map[string]any)["description"] != "" {
		t.Fatalf("empty description should clear: %v isErr=%v", out, isErr)
	}
	out, isErr = f.call(t, "update_category", `{"category_id": "`+catID+`", "name": "Dining"}`)
	if isErr || out["category"].(map[string]any)["name"] != "Dining" {
		t.Fatalf("name-only update failed: %v isErr=%v", out, isErr)
	}

	// Patch with no fields is a typed error; oversized description too.
	out, isErr = f.call(t, "update_category", `{"category_id": "`+catID+`"}`)
	if !isErr || out["error"].(map[string]any)["code"] != "empty_update" {
		t.Fatalf("expected empty_update, got %v isErr=%v", out, isErr)
	}
	out, isErr = f.call(t, "update_category",
		`{"category_id": "`+catID+`", "description": "`+strings.Repeat("x", 501)+`"}`)
	if !isErr || out["error"].(map[string]any)["code"] != "invalid_request" {
		t.Fatalf("expected invalid_request for oversized description, got %v isErr=%v", out, isErr)
	}
}

func TestArchivedCategoryRules(t *testing.T) {
	f := newFixture(t)
	out, _ := f.call(t, "create_category", `{"name": "Old Cat"}`)
	catID := out["category"].(map[string]any)["id"].(string)

	out, isErr := f.call(t, "archive_category", `{"category_id": "`+catID+`"}`)
	if isErr {
		t.Fatalf("archive failed: %v", out)
	}

	// Archived categories cannot back new transactions.
	out, isErr = f.call(t, "create_transaction", `{
		"direction": "money_out", "amount_minor": 100, "currency": "INR",
		"occurred_at": "2026-09-13T10:00:00Z", "category_id": "`+catID+`"
	}`)
	if !isErr || out["error"].(map[string]any)["code"] != "archived_category" {
		t.Fatalf("expected archived_category, got %v isErr=%v", out, isErr)
	}

	// Archive is idempotent.
	out, isErr = f.call(t, "archive_category", `{"category_id": "`+catID+`"}`)
	if isErr {
		t.Fatalf("repeated archive must succeed idempotently: %v", out)
	}

	// Default listing hides archived categories.
	out, _ = f.call(t, "list_categories", `{}`)
	if len(out["categories"].([]any)) != 0 {
		t.Fatalf("archived categories should be hidden by default, got %v", out["categories"])
	}
	out, _ = f.call(t, "list_categories", `{"include_archived": true}`)
	if len(out["categories"].([]any)) != 1 {
		t.Fatalf("archived categories should appear with include_archived, got %v", out["categories"])
	}
}
