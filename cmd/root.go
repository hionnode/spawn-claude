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
	Long: `spawn-claude manages a system-wide Grafana Alloy LaunchDaemon on macOS
(at /etc/alloy/config.alloy, /usr/local/bin/alloy, and
/Library/LaunchDaemons/com.grafana.alloy.plist) and runs ` + "`claude`" + `
with the OTLP env vars needed to emit Claude Code telemetry to it.

Quickstart:
  spawn-claude collector install       # one-time; prompts for sudo
  spawn-claude run -- <claude args>    # every time you'd normally run claude`,
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
