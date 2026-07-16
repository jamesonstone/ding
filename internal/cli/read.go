package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/invoice"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <customer-id>",
		Short: "print a customer's invoices grouped by month",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gdb, _, err := openDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(gdb) }()
			if _, err := ensureCustomer(gdb, args[0]); err != nil {
				return err
			}
			invs, err := db.ListInvoices(gdb, args[0])
			if err != nil {
				return err
			}
			today := time.Now().UTC()
			var b strings.Builder
			for _, g := range invoice.GroupByMonth(invs) {
				fmt.Fprintf(&b, "\n[%s]\n", strings.ToUpper(g.MonthLabel()))
				// tw writes into the strings.Builder, which never errors; the
				// single checked write to the real output happens below.
				tw := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(tw, "  INVOICE\tISSUED\tDUE\tAMOUNT\tSTATUS\tDAYS LATE\tOUTSTANDING")
				for _, inv := range g.Invoices {
					_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%d\t%s\n",
						inv.ID, inv.Issued.Format(dateLayout), inv.Due.Format(dateLayout),
						invoice.FormatCents(inv.AmountCents), inv.Status,
						invoice.DaysLate(inv, today), invoice.FormatCents(invoice.Outstanding(inv)))
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				fmt.Fprintf(&b, "  %s subtotal: %s\n", g.MonthLabel(), invoice.FormatCents(g.MonthTotal))
			}
			fmt.Fprintf(&b, "\nTotal outstanding: %s\n", invoice.FormatCents(invoice.TotalOutstanding(invs)))
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <customer-id>",
		Short: "verify a customer exists and has valid invoice dates",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gdb, _, err := openDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(gdb) }()
			if _, err := ensureCustomer(gdb, args[0]); err != nil {
				return err
			}
			invs, err := db.ListInvoices(gdb, args[0])
			if err != nil {
				return err
			}
			for _, inv := range invs {
				if inv.Issued.IsZero() || inv.Due.IsZero() {
					return fmt.Errorf("invoice %s has a missing date", inv.ID)
				}
				if inv.Due.Before(inv.Issued) {
					return fmt.Errorf("invoice %s due date %s is before issue date %s",
						inv.ID, inv.Due.Format(dateLayout), inv.Issued.Format(dateLayout))
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "ok: %s has %d valid invoice(s)\n", args[0], len(invs))
			return err
		},
	}
}
