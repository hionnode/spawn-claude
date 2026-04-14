package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

var uninstallPurge bool

var collectorUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop the LaunchAgent and remove the plist (use --purge to also wipe binary, config, and logs)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		paths, err := alloy.ResolvePaths()
		if err != nil {
			return err
		}

		_ = alloy.Bootout(ctx, paths.AgentPath)
		if err := os.Remove(paths.AgentPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", paths.AgentPath, err)
		}

		if !uninstallPurge {
			fmt.Println("LaunchAgent removed. Preserved:")
			fmt.Printf("  binary: %s\n", paths.BinPath)
			fmt.Printf("  config: %s\n", paths.ConfigDir)
			fmt.Printf("  logs:   %s\n", paths.LogDir)
			fmt.Println("Pass --purge to remove those too.")
			return nil
		}

		for _, target := range []struct {
			path   string
			remove func(string) error
		}{
			{paths.BinPath, os.Remove},
			{paths.ConfigDir, os.RemoveAll},
			{paths.LogDir, os.RemoveAll},
		} {
			if err := target.remove(target.path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", target.path, err)
			}
		}
		fmt.Println("done. everything removed.")
		return nil
	},
}

func init() {
	collectorUninstallCmd.Flags().BoolVar(&uninstallPurge, "purge", false, "also remove the alloy binary, config dir, and logs")
	collectorCmd.AddCommand(collectorUninstallCmd)
}
