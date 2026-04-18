package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

var collectorRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Hard-restart the LaunchDaemon (launchctl kickstart -k) and wait for /-/ready",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if err := alloy.Kickstart(ctx); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "waiting for %s\n", alloy.ReadyURL)
		if err := alloy.WaitReady(ctx, 10*time.Second); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "READY (%s)\n", alloy.UIBase)
		return nil
	},
}

func init() {
	collectorCmd.AddCommand(collectorRestartCmd)
}
