package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
	"github.com/hionnode/spawn-claude/internal/buildinfo"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print spawn-claude and default Alloy versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("spawn-claude %s\n", buildinfo.Version)
		fmt.Printf("default alloy %s\n", alloy.DefaultVersion)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
