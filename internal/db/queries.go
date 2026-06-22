package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ding/internal/invoice"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a requested customer or invoice does not exist.
var ErrNotFound = errors.New("not found")

// GetCustomer loads a customer by id.
func GetCustomer(gdb *gorm.DB, id string) (invoice.Customer, error) {
	var c invoice.Customer
	err := gdb.Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return invoice.Customer{}, fmt.Errorf("customer %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return invoice.Customer{}, fmt.Errorf("get customer %q: %w", id, err)
	}
	return c, nil
}

// UpsertCustomer inserts or updates a customer by primary key. Used to bootstrap
// a customer from YAML metadata.
func UpsertCustomer(gdb *gorm.DB, c invoice.Customer) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if err := gdb.Save(&c).Error; err != nil {
		return fmt.Errorf("upsert customer %q: %w", c.ID, err)
	}
	return nil
}

// ListInvoices returns a customer's invoices ordered by issue date.
func ListInvoices(gdb *gorm.DB, customerID string) ([]invoice.Invoice, error) {
	var invs []invoice.Invoice
	err := gdb.Where("customer_id = ?", customerID).Order("issued asc, id asc").Find(&invs).Error
	if err != nil {
		return nil, fmt.Errorf("list invoices for %q: %w", customerID, err)
	}
	return invs, nil
}

// GetInvoice loads a single invoice by composite key.
func GetInvoice(gdb *gorm.DB, customerID, invoiceID string) (invoice.Invoice, error) {
	var inv invoice.Invoice
	err := gdb.Where("customer_id = ? AND id = ?", customerID, invoiceID).First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return invoice.Invoice{}, fmt.Errorf("invoice %q/%q: %w", customerID, invoiceID, ErrNotFound)
	}
	if err != nil {
		return invoice.Invoice{}, fmt.Errorf("get invoice %q/%q: %w", customerID, invoiceID, err)
	}
	return inv, nil
}

// InsertInvoice inserts a new invoice, computing the due date from the
// customer's net terms when Due is zero.
func InsertInvoice(gdb *gorm.DB, inv invoice.Invoice) (invoice.Invoice, error) {
	now := time.Now().UTC()
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}
	inv.UpdatedAt = now
	if inv.Status == "" {
		inv.Status = invoice.StatusUnpaid
	}
	if inv.Currency == "" {
		inv.Currency = "USD"
	}
	if err := gdb.Create(&inv).Error; err != nil {
		return invoice.Invoice{}, fmt.Errorf("insert invoice %q/%q: %w", inv.CustomerID, inv.ID, err)
	}
	return inv, nil
}

// MarkPaid records a payment against an invoice.
//
// paidCents is the new cumulative paid amount; a value <= 0 means "paid in
// full". Status becomes "paid" when the paid amount covers the invoice and
// "partial" otherwise. When idemKey is non-empty it is checked first: a
// previously-processed key returns the stored invoice with cached=true and no
// further mutation, making Discord retries safe.
func MarkPaid(gdb *gorm.DB, customerID, invoiceID string, paidDate time.Time, paidCents int64, idemKey string) (result invoice.Invoice, cached bool, err error) {
	if idemKey != "" {
		var existing invoice.Invoice
		e := gdb.Where("idempotency_key = ?", idemKey).First(&existing).Error
		if e == nil {
			return existing, true, nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return invoice.Invoice{}, false, fmt.Errorf("idempotency lookup: %w", e)
		}
	}

	inv, err := GetInvoice(gdb, customerID, invoiceID)
	if err != nil {
		return invoice.Invoice{}, false, err
	}

	if paidCents <= 0 || paidCents >= inv.AmountCents {
		inv.PaidCents = inv.AmountCents
		inv.Status = invoice.StatusPaid
	} else {
		inv.PaidCents = paidCents
		inv.Status = invoice.StatusPartial
	}
	pd := paidDate.UTC()
	inv.PaidDate = &pd
	inv.UpdatedAt = time.Now().UTC()
	if idemKey != "" {
		inv.IdempotencyKey = &idemKey
	}

	if err := gdb.Save(&inv).Error; err != nil {
		return invoice.Invoice{}, false, fmt.Errorf("mark-paid %q/%q: %w", customerID, invoiceID, err)
	}
	return inv, false, nil
}

// ListCustomers returns all customers, used by the monthly send job.
func ListCustomers(gdb *gorm.DB) ([]invoice.Customer, error) {
	var cs []invoice.Customer
	if err := gdb.Order("id asc").Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return cs, nil
}
