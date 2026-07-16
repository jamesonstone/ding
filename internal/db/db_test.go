package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestMigrateCreatesSchemaAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ding_test.db")
	gdb, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = Close(gdb) }()

	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	// Second run must be a no-op.
	if err := RunMigrations(gdb); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	// Exactly one migration recorded.
	var count int64
	if err := gdb.Model(&Migration{}).Count(&count).Error; err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("migration count = %d, want 1", count)
	}

	// Tables present.
	for _, tbl := range []string{"customers", "invoices", "migrations"} {
		if !gdb.Migrator().HasTable(tbl) {
			t.Errorf("table %q missing", tbl)
		}
	}

	// §15 indexes present.
	wantIdx := []string{"idx_invoices_customer_status", "idx_invoices_customer_due", "idx_invoices_idempotency"}
	var names []string
	if err := gdb.Raw("SELECT name FROM sqlite_master WHERE type='index'").Scan(&names).Error; err != nil {
		t.Fatalf("read indexes: %v", err)
	}
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, idx := range wantIdx {
		if !have[idx] {
			t.Errorf("index %q missing (have %v)", idx, names)
		}
	}
}

func TestSeedStatusRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ding_test.db")
	gdb, _ := Open(path)
	defer func() { _ = Close(gdb) }()
	if err := RunMigrations(gdb); err != nil {
		t.Fatal(err)
	}

	if err := UpsertCustomer(gdb, invoice.Customer{ID: "customerx", Name: "Customer X", Email: "c@example.com", SenderName: "J", SenderEmail: "j@example.com", NetDays: 30}); err != nil {
		t.Fatalf("UpsertCustomer: %v", err)
	}

	issued := date("2026-04-15")
	inv := invoice.Invoice{CustomerID: "customerx", ID: "INV-001", Issued: issued, Due: invoice.ComputeDue(issued, 30), AmountCents: 250000}
	if _, err := InsertInvoice(gdb, inv); err != nil {
		t.Fatalf("InsertInvoice: %v", err)
	}

	got, err := ListInvoices(gdb, "customerx")
	if err != nil {
		t.Fatalf("ListInvoices: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d invoices, want 1", len(got))
	}
	if got[0].Status != invoice.StatusUnpaid {
		t.Errorf("status = %q, want unpaid", got[0].Status)
	}
	if got[0].Due.Format("2006-01-02") != "2026-05-15" {
		t.Errorf("due = %s, want 2026-05-15", got[0].Due.Format("2006-01-02"))
	}
}

func TestMarkPaidTransitionsAndIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ding_test.db")
	gdb, _ := Open(path)
	defer func() { _ = Close(gdb) }()
	if err := RunMigrations(gdb); err != nil {
		t.Fatal(err)
	}
	_ = UpsertCustomer(gdb, invoice.Customer{ID: "customerx", Name: "X", Email: "c@e.com", SenderName: "J", SenderEmail: "j@e.com", NetDays: 30})
	issued := date("2026-04-15")
	_, _ = InsertInvoice(gdb, invoice.Invoice{CustomerID: "customerx", ID: "INV-001", Issued: issued, Due: invoice.ComputeDue(issued, 30), AmountCents: 100000})

	// Partial payment.
	inv, cached, err := MarkPaid(gdb, "customerx", "INV-001", date("2026-06-01"), 40000, "")
	if err != nil {
		t.Fatalf("MarkPaid partial: %v", err)
	}
	if cached {
		t.Error("first mark-paid reported cached")
	}
	if inv.Status != invoice.StatusPartial {
		t.Errorf("status = %q, want partial", inv.Status)
	}
	if invoice.Outstanding(inv) != 60000 {
		t.Errorf("outstanding = %d, want 60000", invoice.Outstanding(inv))
	}

	// Full payment via idempotent Discord-style key.
	key := invoice.IdempotencyKey("customerx", "interaction-1", "mark-paid")
	paid, cached, err := MarkPaid(gdb, "customerx", "INV-001", date("2026-06-02"), 0, key)
	if err != nil {
		t.Fatalf("MarkPaid full: %v", err)
	}
	if cached {
		t.Error("full mark-paid reported cached on first apply")
	}
	if paid.Status != invoice.StatusPaid || invoice.Outstanding(paid) != 0 {
		t.Errorf("status=%q outstanding=%d, want paid/0", paid.Status, invoice.Outstanding(paid))
	}

	// Replaying the same key returns the cached row with no further mutation.
	again, cached, err := MarkPaid(gdb, "customerx", "INV-001", date("2030-01-01"), 99999, key)
	if err != nil {
		t.Fatalf("MarkPaid replay: %v", err)
	}
	if !cached {
		t.Error("idempotent replay not reported as cached")
	}
	if again.PaidCents != paid.PaidCents || again.Status != paid.Status {
		t.Errorf("replay mutated row: %+v vs %+v", again, paid)
	}
}
