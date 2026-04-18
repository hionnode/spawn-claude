package cmd

import "github.com/spf13/cobra"

var collectorCmd = &cobra.Command{
	Use:   "collector",
	Short: "Manage the local Grafana Alloy collector",
	Long:  `Install, configure, and monitor the system-wide Grafana Alloy LaunchDaemon that receives OTLP telemetry from Claude Code.`,
}

func init() {
	rootCmd.AddCommand(collectorCmd)
}
