package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

var collectorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the LaunchDaemon is loaded and the Alloy UI is ready",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		status, err := alloy.Status(ctx)
		if err != nil {
			return err
		}

		if !status.Loaded {
			fmt.Println("service:  not loaded")
		} else if status.Running {
			fmt.Printf("service:  loaded (PID %d, last exit %d)\n", status.PID, status.LastExit)
		} else {
			fmt.Printf("service:  loaded (not running, last exit %d)\n", status.LastExit)
		}

		readyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		ok, code, rerr := alloy.CheckReady(readyCtx)
		switch {
		case ok:
			fmt.Printf("ready:    ok (%s returned %d)\n", alloy.ReadyURL, code)
		case rerr != nil:
			fmt.Printf("ready:    fail (%s: %v)\n", alloy.ReadyURL, rerr)
		default:
			fmt.Printf("ready:    fail (%s returned %d)\n", alloy.ReadyURL, code)
		}

		if !status.Loaded || !status.Running || !ok {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	collectorCmd.AddCommand(collectorStatusCmd)
}
