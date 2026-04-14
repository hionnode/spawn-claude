package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var verbose bool

var rootCmd = &cobra.Command{
	Use:   "spawn-claude",
	Short: "Run Claude Code with OTLP telemetry via a local Grafana Alloy collector",
	Long: `spawn-claude manages a per-user Grafana Alloy collector on macOS and will
(in upcoming releases) wrap ` + "`claude`" + ` with the OTLP env vars needed to emit
Claude Code telemetry to a configurable backend.

This release ships only the collector lifecycle commands; see
` + "`spawn-claude collector --help`" + `.`,
	SilenceUsage:  true,
	SilenceErrors: false,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	cobra.OnInitialize(setupLogging)
}

func setupLogging() {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}
