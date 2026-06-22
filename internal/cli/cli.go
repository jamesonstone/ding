// Package cli implements the ding command-line surface (cobra). All commands
// read configuration from the environment (DING_DB_PATH and friends) and use
// the SQLite database as the source of truth for invoices.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jamesonstone/ding/internal/config"
	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/invoice"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// customersDir is the directory holding per-customer YAML metadata, bound to the
// root --customers-dir flag.
var customersDir = "customers"

// Execute runs the root command. A non-nil error exits with status 1
// (fail-closed) after cobra prints it.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "ding",
		Short:         "🔔 invoice tracking and automated payment reminders",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().StringVar(&customersDir, "customers-dir", "customers",
		"directory holding per-customer YAML metadata")
	root.AddCommand(
		newMigrateCmd(),
		newSendCmd(),
		newMarkPaidCmd(),
		newStatusCmd(),
		newValidateCmd(),
		newSeedCmd(),
	)
	return root
}

// openDB loads runtime config and opens the SQLite database.
func openDB() (*gorm.DB, config.Env, error) {
	env, err := config.LoadEnv()
	if err != nil {
		return nil, config.Env{}, err
	}
	gdb, err := db.Open(env.DBPath)
	if err != nil {
		return nil, config.Env{}, err
	}
	return gdb, env, nil
}

// ensureCustomer returns the customer, hydrating it from
// <customers-dir>/<id>-ding.yaml when it is not yet in the database. A customer
// that exists in neither place is an error (fail-closed).
func ensureCustomer(gdb *gorm.DB, id string) (invoice.Customer, error) {
	cust, err := db.GetCustomer(gdb, id)
	if err == nil {
		return cust, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return invoice.Customer{}, err
	}
	cfgPath := filepath.Join(customersDir, id+"-ding.yaml")
	cfg, cfgErr := config.LoadConfig(cfgPath)
	if cfgErr != nil {
		return invoice.Customer{}, fmt.Errorf("customer %q not in database and no metadata at %s: %w", id, cfgPath, cfgErr)
	}
	c := cfg.ToCustomer()
	if err := db.UpsertCustomer(gdb, c); err != nil {
		return invoice.Customer{}, err
	}
	return c, nil
}
