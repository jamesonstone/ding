package cli

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/invoice"
	"github.com/spf13/cobra"
)

const dateLayout = "2006-01-02"

func newSeedCmd() *cobra.Command {
	var id, issued string
	var amount int64
	c := &cobra.Command{
		Use:   "seed <customer-id>",
		Short: "insert an invoice for a customer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gdb, _, err := openDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(gdb) }()
			cust, err := ensureCustomer(gdb, args[0])
			if err != nil {
				return err
			}
			iss, err := time.Parse(dateLayout, issued)
			if err != nil {
				return fmt.Errorf("invalid --issued %q (want YYYY-MM-DD): %w", issued, err)
			}
			if amount <= 0 {
				return fmt.Errorf("--amount must be a positive number of cents")
			}
			inv := invoice.Invoice{
				CustomerID:  args[0],
				ID:          id,
				Issued:      iss,
				Due:         invoice.ComputeDue(iss, cust.NetDays),
				AmountCents: amount,
				Currency:    "USD",
				Status:      invoice.StatusUnpaid,
			}
			if _, err := db.InsertInvoice(gdb, inv); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "seeded %s/%s for %s, due %s\n",
				args[0], id, invoice.FormatCents(amount), inv.Due.Format(dateLayout))
			return err
		},
	}
	c.Flags().StringVar(&id, "id", "", "invoice id, e.g. INV-2026-001 (required)")
	c.Flags().StringVar(&issued, "issued", "", "issue date YYYY-MM-DD (required)")
	c.Flags().Int64Var(&amount, "amount", 0, "amount in cents (required)")
	_ = c.MarkFlagRequired("id")
	_ = c.MarkFlagRequired("issued")
	_ = c.MarkFlagRequired("amount")
	return c
}

func newMarkPaidCmd() *cobra.Command {
	var id, dateStr string
	var cents int64
	c := &cobra.Command{
		Use:   "mark-paid <customer-id>",
		Short: "record a payment against an invoice",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gdb, _, err := openDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(gdb) }()
			paid, err := time.Parse(dateLayout, dateStr)
			if err != nil {
				return fmt.Errorf("invalid --date %q (want YYYY-MM-DD): %w", dateStr, err)
			}
			// CLI has no Discord interaction id, so no idempotency key is used.
			inv, _, err := db.MarkPaid(gdb, args[0], id, paid, cents, "")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s/%s is now %s, outstanding %s\n",
				args[0], inv.ID, inv.Status, invoice.FormatCents(invoice.Outstanding(inv)))
			return err
		},
	}
	c.Flags().StringVar(&id, "id", "", "invoice id (required)")
	c.Flags().StringVar(&dateStr, "date", "", "payment date YYYY-MM-DD (required)")
	c.Flags().Int64Var(&cents, "cents", 0, "amount paid in cents (omit for paid in full)")
	_ = c.MarkFlagRequired("id")
	_ = c.MarkFlagRequired("date")
	return c
}
