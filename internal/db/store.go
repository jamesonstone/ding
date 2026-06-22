package db

import (
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
	"gorm.io/gorm"
)

// Store adapts a *gorm.DB to the small data-access interfaces consumed by the
// discord and sendjob packages, so those packages do not depend on GORM.
type Store struct{ DB *gorm.DB }

// NewStore wraps a database handle.
func NewStore(gdb *gorm.DB) Store { return Store{DB: gdb} }

// GetCustomer loads a customer by id.
func (s Store) GetCustomer(id string) (invoice.Customer, error) { return GetCustomer(s.DB, id) }

// ListInvoices returns a customer's invoices ordered by issue date.
func (s Store) ListInvoices(customerID string) ([]invoice.Invoice, error) {
	return ListInvoices(s.DB, customerID)
}

// ListCustomers returns all customers.
func (s Store) ListCustomers() ([]invoice.Customer, error) { return ListCustomers(s.DB) }

// InsertInvoice inserts a new invoice.
func (s Store) InsertInvoice(inv invoice.Invoice) (invoice.Invoice, error) {
	return InsertInvoice(s.DB, inv)
}

// MarkPaid records a payment with idempotency handling.
func (s Store) MarkPaid(customerID, invoiceID string, paidDate time.Time, paidCents int64, idemKey string) (invoice.Invoice, bool, error) {
	return MarkPaid(s.DB, customerID, invoiceID, paidDate, paidCents, idemKey)
}
