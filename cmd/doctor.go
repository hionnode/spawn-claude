package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/doctor"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run end-to-end health checks against the installed collector",
	Long: `doctor runs a battery of checks that verify a spawn-claude install is
healthy end-to-end: claude is on PATH, the alloy binary is installed, the
LaunchDaemon is loaded and running, the UI /-/ready endpoint responds, and
the OTLP receiver ports (4317/4318) are accepting connections.

Exits 0 on success, 1 if any check failed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		results := doctor.Run(cmd.Context())
		for _, r := range results {
			fmt.Println(r)
		}
		if doctor.AnyFailed(results) {
			fmt.Fprintln(os.Stderr, "\nOne or more checks failed. Address the hints above and re-run `spawn-claude doctor`.")
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
