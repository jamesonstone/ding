package email

import (
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}
func ptr(t time.Time) *time.Time { return &t }

func sample() (invoice.Customer, []invoice.Invoice) {
	c := invoice.Customer{Name: "Customer X", SenderName: "Jameson Stone"}
	invs := []invoice.Invoice{
		{ID: "INV-2026-004", Issued: date("2026-03-20"), Due: date("2026-04-19"), AmountCents: 150000, PaidCents: 75000, Status: invoice.StatusPartial},
		{ID: "INV-2026-002", Issued: date("2026-05-10"), Due: date("2026-06-09"), AmountCents: 180000, PaidCents: 180000, Status: invoice.StatusPaid, PaidDate: ptr(date("2026-06-08"))},
	}
	return c, invs
}

func TestRenderHTML(t *testing.T) {
	c, invs := sample()
	html, _, err := Render(BuildData(c, invs, date("2026-06-22")))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Inline CSS only — Gmail strips <style> blocks.
	if strings.Contains(strings.ToLower(html), "<style") {
		t.Error("HTML contains a <style> block; must be inline CSS only")
	}
	// Late invoice (INV-2026-004 is 64 days late) is colored.
	if !strings.Contains(html, "#d9534f") {
		t.Error("late rows are not colored #d9534f")
	}
	// Grand total present and correct (only the partial invoice is outstanding).
	if !strings.Contains(html, "Total outstanding") {
		t.Error("missing grand-total row")
	}
	if !strings.Contains(html, "$750.00") {
		t.Error("expected outstanding $750.00 in HTML")
	}
	// Month subtotal label present.
	if !strings.Contains(html, "subtotal") {
		t.Error("missing month subtotal")
	}
}

func TestRenderText(t *testing.T) {
	c, invs := sample()
	_, text, err := Render(BuildData(c, invs, date("2026-06-22")))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(text, "[MARCH 2026]") {
		t.Errorf("plaintext missing [MARCH 2026] header:\n%s", text)
	}
	if !strings.Contains(text, "TOTAL OUTSTANDING: $750.00") {
		t.Errorf("plaintext missing grand total:\n%s", text)
	}
}

func TestSubject(t *testing.T) {
	got, err := Subject("Invoice status — {{.Month}} {{.Year}}", date("2026-06-22"))
	if err != nil {
		t.Fatalf("Subject: %v", err)
	}
	if got != "Invoice status — June 2026" {
		t.Errorf("subject = %q", got)
	}
}
