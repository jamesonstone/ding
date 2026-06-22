// Package invoice holds the core domain types and the pure business logic
// (lateness, outstanding balance, month grouping, idempotency keys) that the
// rest of ding builds on. It has no database or network dependencies so the
// logic can be exercised with fast table tests.
package invoice

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Invoice status values. Stored as text and constrained by a CHECK in the
// schema (see internal/db migrations).
const (
	StatusUnpaid  = "unpaid"
	StatusPartial = "partial"
	StatusPaid    = "paid"
)

// Customer is a billable entity. All mutable state lives in the database; the
// YAML config (internal/config) only carries the same metadata for bootstrap.
type Customer struct {
	ID                    string    `gorm:"primaryKey;column:id"`
	Name                  string    `gorm:"column:name;not null"`
	Email                 string    `gorm:"column:email;not null"`
	SenderName            string    `gorm:"column:sender_name;not null"`
	SenderEmail           string    `gorm:"column:sender_email;not null"`
	NetDays               int       `gorm:"column:net_days;default:30"`
	ReminderThresholdDays int       `gorm:"column:reminder_threshold_days;default:0"`
	DiscordWebhookSecret  string    `gorm:"column:discord_webhook_secret"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	Invoices              []Invoice `gorm:"foreignKey:CustomerID;references:ID;constraint:OnDelete:CASCADE"`
}

// TableName pins the GORM table name regardless of struct naming.
func (Customer) TableName() string { return "customers" }

// Invoice is a single billed amount for a customer. The identifier is unique
// per customer (composite primary key customer_id+id), matching how the CLI and
// Discord commands address invoices ("<customer> --id INV-...").
type Invoice struct {
	CustomerID     string     `gorm:"primaryKey;column:customer_id;index:idx_invoices_customer_status,priority:1;index:idx_invoices_customer_due,priority:1"`
	ID             string     `gorm:"primaryKey;column:id"`
	Issued         time.Time  `gorm:"column:issued;not null"`
	Due            time.Time  `gorm:"column:due;not null;index:idx_invoices_customer_due,priority:2"`
	AmountCents    int64      `gorm:"column:amount_cents;not null"`
	Currency       string     `gorm:"column:currency;default:USD"`
	Status         string     `gorm:"column:status;index:idx_invoices_customer_status,priority:2"`
	PaidDate       *time.Time `gorm:"column:paid_date"`
	PaidCents      int64      `gorm:"column:paid_cents;default:0"`
	IdempotencyKey *string    `gorm:"column:idempotency_key;uniqueIndex:idx_invoices_idempotency"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

// TableName pins the GORM table name regardless of struct naming.
func (Invoice) TableName() string { return "invoices" }

// DaysLate reports how many days past due an invoice is.
//
//   - paid with a paid date: historical lateness (paid_date - due), floored at 0.
//   - not paid: current lateness (today - due), floored at 0.
//   - paid without a paid date: 0.
func DaysLate(inv Invoice, today time.Time) int {
	if inv.Status == StatusPaid {
		if inv.PaidDate != nil {
			return maxInt(0, dayDiff(inv.Due, *inv.PaidDate))
		}
		return 0
	}
	return maxInt(0, dayDiff(inv.Due, today))
}

// Outstanding reports the unpaid balance in cents. Paid invoices owe nothing;
// otherwise it is the amount minus whatever has been paid.
func Outstanding(inv Invoice) int64 {
	if inv.Status == StatusPaid {
		return 0
	}
	return inv.AmountCents - inv.PaidCents
}

// ComputeDue returns the due date for an invoice issued on the given date under
// net-N terms.
func ComputeDue(issued time.Time, netDays int) time.Time {
	return issued.AddDate(0, 0, netDays)
}

// IdempotencyKey is the deterministic hash used to deduplicate mutating Discord
// interactions: sha256(customerID|interactionID|action), hex-encoded.
func IdempotencyKey(customerID, interactionID, action string) string {
	sum := sha256.Sum256([]byte(customerID + "|" + interactionID + "|" + action))
	return hex.EncodeToString(sum[:])
}

// MonthGroup is a set of invoices that share an issued month, plus the
// outstanding subtotal for that month.
type MonthGroup struct {
	Year       int
	Month      time.Month
	Invoices   []Invoice
	MonthTotal int64 // sum of Outstanding across Invoices, in cents
}

// MonthLabel renders the group's month for templates, e.g. "March 2026".
func (g MonthGroup) MonthLabel() string {
	return fmt.Sprintf("%s %d", g.Month.String(), g.Year)
}

// GroupByMonth groups invoices by their issued month in chronological order and
// computes each month's outstanding subtotal.
func GroupByMonth(invoices []Invoice) []MonthGroup {
	type key struct {
		year  int
		month time.Month
	}
	index := map[key]int{}
	groups := []MonthGroup{}
	for _, inv := range invoices {
		k := key{inv.Issued.Year(), inv.Issued.Month()}
		i, ok := index[k]
		if !ok {
			i = len(groups)
			index[k] = i
			groups = append(groups, MonthGroup{Year: k.year, Month: k.month})
		}
		groups[i].Invoices = append(groups[i].Invoices, inv)
		groups[i].MonthTotal += Outstanding(inv)
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].Year != groups[b].Year {
			return groups[a].Year < groups[b].Year
		}
		return groups[a].Month < groups[b].Month
	})
	return groups
}

// TotalOutstanding sums the outstanding balance across all invoices, in cents.
func TotalOutstanding(invoices []Invoice) int64 {
	var total int64
	for _, inv := range invoices {
		total += Outstanding(inv)
	}
	return total
}

// FormatCents renders a cent amount as a currency string, e.g. 150000 -> "$1,500.00".
func FormatCents(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	dollars := cents / 100
	rem := cents % 100
	out := fmt.Sprintf("%d.%02d", dollars, rem)
	out = withThousands(out)
	if neg {
		return "-$" + out
	}
	return "$" + out
}

// withThousands inserts comma separators into the integer part of a "D.CC" string.
func withThousands(s string) string {
	dot := strings.IndexByte(s, '.')
	intPart, frac := s[:dot], s[dot:]
	var b strings.Builder
	n := len(intPart)
	for i, c := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + frac
}

// dayDiff returns the number of whole calendar days from `from` to `to`,
// computed on UTC date boundaries so wall-clock time and DST do not skew it.
func dayDiff(from, to time.Time) int {
	f := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	t := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(t.Sub(f).Hours() / 24)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
