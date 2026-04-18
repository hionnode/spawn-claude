package cmd

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

var uninstallPurge bool

var collectorUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Bootout the LaunchDaemon and remove the plist + binary (use --purge to also wipe config, data, and logs)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		paths, err := alloy.ResolvePaths()
		if err != nil {
			return err
		}

		var b bytes.Buffer
		fmt.Fprintf(&b, "launchctl bootout system/%s 2>/dev/null || true\n", alloy.Label)
		fmt.Fprintf(&b, "rm -f %q %q\n", paths.AgentPath, paths.BinPath)
		if uninstallPurge {
			fmt.Fprintf(&b, "rm -rf %q %q %q\n", paths.ConfigDir, "/var/lib/alloy", paths.LogDir)
		}
		if err := alloy.SudoShell(ctx, b.String()); err != nil {
			return err
		}

		if !uninstallPurge {
			fmt.Println("LaunchDaemon + binary removed. Preserved:")
			fmt.Printf("  config: %s\n", paths.ConfigDir)
			fmt.Printf("  data:   %s\n", paths.DataDir)
			fmt.Printf("  logs:   %s\n", paths.LogDir)
			fmt.Println("Pass --purge to remove those too.")
			return nil
		}
		fmt.Println("done. everything removed.")
		return nil
	},
}

func init() {
	collectorUninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, "also remove /etc/alloy, /var/lib/alloy, and /var/log/alloy")
	collectorCmd.AddCommand(collectorUninstallCmd)
}
