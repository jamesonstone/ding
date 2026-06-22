package cli

import (
	"fmt"

	"github.com/jamesonstone/ding/internal/db"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "create or update the database schema (run once before first use)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			gdb, _, err := openDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(gdb) }()
			if err := db.RunMigrations(gdb); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "migrations applied")
			return err
		},
	}
}
