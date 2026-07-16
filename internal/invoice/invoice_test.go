package invoice

import (
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(t time.Time) *time.Time { return &t }

// sampleInvoices reproduces the four representative invoices from SPEC §4.
func sampleInvoices() []Invoice {
	return []Invoice{
		{ID: "INV-2026-004", CustomerID: "customerx", Issued: date("2026-03-20"), Due: date("2026-04-19"), AmountCents: 150000, PaidCents: 75000, Status: StatusPartial},
		{ID: "INV-2026-001", CustomerID: "customerx", Issued: date("2026-04-15"), Due: date("2026-05-15"), AmountCents: 250000, Status: StatusUnpaid},
		{ID: "INV-2026-002", CustomerID: "customerx", Issued: date("2026-05-10"), Due: date("2026-06-09"), AmountCents: 180000, PaidCents: 180000, Status: StatusPaid, PaidDate: ptr(date("2026-06-08"))},
		{ID: "INV-2026-003", CustomerID: "customerx", Issued: date("2026-06-01"), Due: date("2026-07-01"), AmountCents: 320000, Status: StatusUnpaid},
	}
}

func TestDaysLateAndOutstandingMatchSimulation(t *testing.T) {
	today := date("2026-06-22")
	invoices := sampleInvoices()

	wantLate := []int{64, 38, 0, 0}
	wantOut := []int64{75000, 250000, 0, 320000}

	for i, inv := range invoices {
		if got := DaysLate(inv, today); got != wantLate[i] {
			t.Errorf("%s DaysLate = %d, want %d", inv.ID, got, wantLate[i])
		}
		if got := Outstanding(inv); got != wantOut[i] {
			t.Errorf("%s Outstanding = %d, want %d", inv.ID, got, wantOut[i])
		}
	}

	if got := TotalOutstanding(invoices); got != 645000 {
		t.Errorf("TotalOutstanding = %d, want 645000 ($6,450.00)", got)
	}
}

func TestDaysLatePaidWithoutDate(t *testing.T) {
	inv := Invoice{Status: StatusPaid, Due: date("2026-01-01")}
	if got := DaysLate(inv, date("2026-06-22")); got != 0 {
		t.Errorf("paid without paid_date DaysLate = %d, want 0", got)
	}
}

func TestComputeDue(t *testing.T) {
	if got := ComputeDue(date("2026-04-15"), 30); !got.Equal(date("2026-05-15")) {
		t.Errorf("ComputeDue = %s, want 2026-05-15", got.Format("2006-01-02"))
	}
}

func TestIdempotencyKeyDeterministic(t *testing.T) {
	a := IdempotencyKey("customerx", "interaction-1", "mark-paid")
	b := IdempotencyKey("customerx", "interaction-1", "mark-paid")
	c := IdempotencyKey("customerx", "interaction-2", "mark-paid")
	if a != b {
		t.Errorf("idempotency key not deterministic: %s != %s", a, b)
	}
	if a == c {
		t.Error("idempotency key collided across distinct interaction IDs")
	}
	if len(a) != 64 {
		t.Errorf("sha256 hex key length = %d, want 64", len(a))
	}
}

func TestGroupByMonthChronological(t *testing.T) {
	groups := GroupByMonth(sampleInvoices())
	if len(groups) != 4 {
		t.Fatalf("got %d month groups, want 4", len(groups))
	}
	wantLabels := []string{"March 2026", "April 2026", "May 2026", "June 2026"}
	for i, g := range groups {
		if g.MonthLabel() != wantLabels[i] {
			t.Errorf("group %d label = %q, want %q", i, g.MonthLabel(), wantLabels[i])
		}
	}
	// May group is the fully-paid invoice; its outstanding subtotal is zero.
	if groups[2].MonthTotal != 0 {
		t.Errorf("May MonthTotal = %d, want 0", groups[2].MonthTotal)
	}
}

func TestFormatCents(t *testing.T) {
	cases := map[int64]string{
		0:       "$0.00",
		75000:   "$750.00",
		150000:  "$1,500.00",
		320000:  "$3,200.00",
		645000:  "$6,450.00",
		1234567: "$12,345.67",
		-50000:  "-$500.00",
	}
	for in, want := range cases {
		if got := FormatCents(in); got != want {
			t.Errorf("FormatCents(%d) = %q, want %q", in, got, want)
		}
	}
}
