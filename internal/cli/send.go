package cli

import (
	"github.com/jamesonstone/ding/internal/db"
	"github.com/jamesonstone/ding/internal/sendjob"
	"github.com/spf13/cobra"
)

func newSendCmd() *cobra.Command {
	var dryRun bool
	c := &cobra.Command{
		Use:   "send <customer-id>",
		Short: "render and send the monthly invoice summary (read-only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gdb, env, err := openDB()
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(gdb) }()
			// Hydrate the customer from metadata if needed; fail closed otherwise.
			if _, err := ensureCustomer(gdb, args[0]); err != nil {
				return err
			}
			deps := sendjob.Deps{DB: gdb, Env: env}
			return sendjob.RunForCustomer(cmd.Context(), deps, args[0], dryRun, cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "render to stdout without sending email or posting to Discord")
	return c
}
