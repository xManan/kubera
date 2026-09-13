package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
)

// openTestDB opens and migrates a fresh temporary database.
func openTestDB(t *testing.T) (*sql.DB, testRepos) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "kubera.db"), 5000)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, testRepos{
		Txns:  txRepo{},
		Cats:  catRepo{},
		Audit: auditRepo{},
		Rep:   reportRepo{},
	}
}

type testRepos struct {
	Txns  txRepo
	Cats  catRepo
	Audit auditRepo
	Rep   reportRepo
}

func mustCategory(t *testing.T, ctx context.Context, db *sql.DB, repos testRepos, name string) domain.Category {
	t.Helper()
	c := domain.Category{
		ID: domain.CategoryID("cat_" + name), Name: name,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := repos.Cats.Create(ctx, tx, c); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return c
}

func mustTx(t *testing.T, ctx context.Context, db *sql.DB, repos testRepos, cat domain.Category, dir domain.Direction, amount int64, occurred time.Time, desc, ref string) domain.Transaction {
	t.Helper()
	now := time.Now().UTC()
	x := domain.Transaction{
		ID: domain.TransactionID("txn_" + desc + "_" + occurred.Format("150405.000")), Direction: dir,
		AmountMinor: amount, Currency: "INR", OccurredAt: occurred, CategoryID: cat.ID,
		Description: desc, ReferenceID: ref, CreatedAt: now, UpdatedAt: now,
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := repos.Txns.Insert(ctx, tx, x); err != nil {
		t.Fatal(err)
	}
	if err := repos.Audit.Append(ctx, tx, domain.AuditEvent{
		ID: domain.AuditEventID("aud_" + string(x.ID)), EntityType: domain.EntityTypeTransaction,
		EntityID: string(x.ID), Operation: domain.OpCreated, OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return x
}

func candidate(x domain.Transaction) duplicate.Candidate {
	return duplicate.Candidate{
		Direction: x.Direction, AmountMinor: x.AmountMinor, Currency: x.Currency,
		OccurredAt: x.OccurredAt, NormalizedDescription: duplicate.NormalizeText(x.Description),
		ReferenceID: x.ReferenceID,
	}
}

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "kubera.db")
	if _, err := Open(path, 5000); err != nil {
		t.Fatal(err)
	}
	// (open+migrate twice below; first handle intentionally leaks until t.Cleanup)
	db, err := Open(path, 5000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatalf("reopening an already migrated database must be safe: %v", err)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDB(t)
	_, err := db.ExecContext(ctx,
		"INSERT INTO transactions (id, direction, amount_minor, currency, occurred_at, category_id, created_at, updated_at) VALUES ('x', 'money_in', 1, 'INR', '2026-01-01T00:00:00Z', 'missing', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')")
	if err == nil {
		t.Fatal("insert with missing category should violate foreign key")
	}
}

func TestDuplicateDetection(t *testing.T) {
	ctx := context.Background()
	db, repos := openTestDB(t)
	cat := mustCategory(t, ctx, db, repos, "food")
	base := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

	x := mustTx(t, ctx, db, repos, cat, domain.MoneyOut, 19800, base, "EATCLUB BRANDS PRIVATE", "")

	// Same fields within tolerance → duplicate (repo compares pre-normalized
	// candidate values; the service layer normalizes).
	dup, isDup, err := repos.Txns.FindLikelyDuplicate(ctx, db,
		duplicate.Candidate{
			Direction: domain.MoneyOut, AmountMinor: 19800, Currency: "INR",
			OccurredAt:            base.Add(2 * time.Minute),
			NormalizedDescription: duplicate.NormalizeText("EATCLUB   brands private"),
		}, duplicate.Tolerance)
	if err != nil || !isDup || dup.ID != x.ID {
		t.Fatalf("expected duplicate of %s, got %v isDup=%v err=%v", x.ID, dup.ID, isDup, err)
	}

	// Same amount at a different time → not a duplicate.
	_, isDup, err = repos.Txns.FindLikelyDuplicate(ctx, db,
		duplicate.Candidate{
			Direction: domain.MoneyOut, AmountMinor: 19800, Currency: "INR",
			OccurredAt: base.Add(30 * time.Minute), NormalizedDescription: "eatclub brands private",
		}, duplicate.Tolerance)
	if err != nil || isDup {
		t.Fatalf("same amount 30min later must not duplicate: isDup=%v err=%v", isDup, err)
	}

	// Reference ID wins even if description differs.
	y := mustTx(t, ctx, db, repos, cat, domain.MoneyOut, 19900, base.Add(time.Hour), "Something Else", "REF-1")
	refMatch, isDup, err := repos.Txns.FindLikelyDuplicate(ctx, db,
		duplicate.Candidate{
			Direction: domain.MoneyOut, AmountMinor: 99999, Currency: "INR",
			OccurredAt: base.Add(2 * time.Hour), NormalizedDescription: "totally different",
			ReferenceID: "REF-1",
		}, duplicate.Tolerance)
	if err != nil || !isDup || refMatch.ID != y.ID {
		t.Fatalf("reference match should find %s, got %s: isDup=%v err=%v", y.ID, refMatch.ID, isDup, err)
	}

	// Voided transactions are excluded from duplicate matching.
	vtx := mustTx(t, ctx, db, repos, cat, domain.MoneyOut, 500, base, "Coffee", "")
	tx2, _ := db.BeginTx(ctx, nil)
	if err := repos.Txns.Void(ctx, tx2, vtx.ID, base.Add(time.Minute), "test"); err != nil {
		t.Fatal(err)
	}
	tx2.Commit()
	_, isDup, err = repos.Txns.FindLikelyDuplicate(ctx, db,
		duplicate.Candidate{
			Direction: domain.MoneyOut, AmountMinor: 500, Currency: "INR",
			OccurredAt: base, NormalizedDescription: "coffee",
		}, duplicate.Tolerance)
	if err != nil || isDup {
		t.Fatalf("voided transactions must not match: isDup=%v err=%v", isDup, err)
	}
}

func TestVoidAndReportsExcludeVoided(t *testing.T) {
	ctx := context.Background()
	db, repos := openTestDB(t)
	cat := mustCategory(t, ctx, db, repos, "salary")
	base := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	mustTx(t, ctx, db, repos, cat, domain.MoneyOut, 10000, base, "a", "")
	v := mustTx(t, ctx, db, repos, cat, domain.MoneyOut, 20000, base.Add(time.Minute), "b", "")
	mustTx(t, ctx, db, repos, cat, domain.MoneyIn, 50000, base.Add(2*time.Minute), "c", "")

	tx, _ := db.BeginTx(ctx, nil)
	if err := repos.Txns.Void(ctx, tx, v.ID, base.Add(time.Hour), "wrong parse"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Void is not applied twice (guard: voided_at IS NULL).
	tx2, _ := db.BeginTx(ctx, nil)
	if err := repos.Txns.Void(ctx, tx2, v.ID, base.Add(2*time.Hour), ""); err == nil {
		t.Fatal("second void must not affect rows")
	}
	tx2.Rollback()

	totals, err := repos.Rep.CurrencyTotals(ctx, db, base, base.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 || totals[0].MoneyOutMinor != 10000 || totals[0].MoneyInMinor != 50000 {
		t.Fatalf("voided transaction must be excluded: %+v", totals)
	}

	cats, err := repos.Rep.CategoryTotals(ctx, db, base, base.Add(24*time.Hour))
	if err != nil || len(cats) != 2 {
		t.Fatalf("expected 2 category rows (in+out), got %+v err=%v", cats, err)
	}

	// Deterministic ordering: currency asc, direction asc, total desc.
	if cats[0].Direction != domain.MoneyIn || cats[1].Direction != domain.MoneyOut {
		t.Fatalf("unexpected ordering: %+v", cats)
	}
}

func TestListCursorPagination(t *testing.T) {
	ctx := context.Background()
	db, repos := openTestDB(t)
	cat := mustCategory(t, ctx, db, repos, "x")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		mustTx(t, ctx, db, repos, cat, domain.MoneyOut, 100, base.Add(time.Duration(i)*time.Minute), "t", "")
	}
	f := domain.TransactionFilter{Limit: 2}
	page1, p1, err := repos.Txns.List(ctx, db, f)
	if err != nil || len(page1) != 2 || !p1.HasMore {
		t.Fatalf("page1: %d items, more=%v, err=%v", len(page1), p1.HasMore, err)
	}
	f.Cursor = p1.NextCursor
	page2, p2, err := repos.Txns.List(ctx, db, f)
	if err != nil || len(page2) != 2 || !p2.HasMore {
		t.Fatalf("page2: %d items, more=%v, err=%v", len(page2), p2.HasMore, err)
	}
	// Newest first, no overlap between pages.
	if !page1[0].OccurredAt.After(page2[0].OccurredAt) {
		t.Fatal("expected newest-first ordering across pages")
	}
	f.Cursor = p2.NextCursor
	page3, p3, err := repos.Txns.List(ctx, db, f)
	if err != nil || len(page3) != 1 || p3.HasMore {
		t.Fatalf("page3: %d items, more=%v, err=%v", len(page3), p3.HasMore, err)
	}
}

func TestCategoryUniqueAmongActive(t *testing.T) {
	ctx := context.Background()
	db, repos := openTestDB(t)
	mustCategory(t, ctx, db, repos, "Food")

	// Same normalized name (case/space differences) is rejected.
	tx, _ := db.BeginTx(ctx, nil)
	defer tx.Rollback()
	err := repos.Cats.Create(ctx, tx, domain.Category{
		ID: "cat_food2", Name: "  FOOD  ", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err == nil {
		t.Fatal("duplicate active category name must be rejected")
	}
}

func TestConcurrentDuplicateCreation(t *testing.T) {
	ctx := context.Background()
	db, repos := openTestDB(t)
	cat := mustCategory(t, ctx, db, repos, "transport")
	base := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)

	// 8 workers race to insert the identical transaction; duplicate checks
	// and inserts serialize under BEGIN IMMEDIATE, so exactly one wins.
	var wg sync.WaitGroup
	wins := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return
			}
			defer tx.Rollback()
			x := domain.Transaction{
				Direction: domain.MoneyOut, AmountMinor: 7000, Currency: "INR",
				OccurredAt: base, CategoryID: cat.ID, Description: "Metro",
				CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			if _, isDup, err := repos.Txns.FindLikelyDuplicate(ctx, tx, candidate(x), duplicate.Tolerance); err != nil || isDup {
				return
			}
			x.ID = domain.TransactionID(newTestID())
			if err := repos.Txns.Insert(ctx, tx, x); err != nil {
				return
			}
			if tx.Commit() == nil {
				wins <- string(x.ID)
			}
		}()
	}
	wg.Wait()
	close(wins)
	n := 0
	for range wins {
		n++
	}
	if n != 1 {
		t.Fatalf("exactly one concurrent insert should win, got %d", n)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM transactions WHERE amount_minor = 7000").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 stored transaction, got %d", count)
	}
}

var testIDCounter atomic.Int64

func newTestID() string {
	return "txn_test_" + strconv.FormatInt(testIDCounter.Add(1), 10)
}
