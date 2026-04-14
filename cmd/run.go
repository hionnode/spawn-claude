package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
	"github.com/hionnode/spawn-claude/internal/otel"
)

var (
	runSkipReadyCheck bool
	runPrintEnv       bool
)

var runCmd = &cobra.Command{
	Use:   "run [-- claude args...]",
	Short: "Run `claude` with OTLP telemetry routed through the local Alloy collector",
	Long: `run executes the ` + "`claude`" + ` binary with the environment variables needed to
emit OTLP telemetry (metrics + logs) to the spawn-claude-managed Alloy
collector on http://127.0.0.1:4318. Any arguments after ` + "`--`" + ` are forwarded
verbatim to claude.

The collector must already be running (` + "`spawn-claude collector install`" + ` once,
then ` + "`spawn-claude collector status`" + ` to verify). Use --skip-ready-check to
bypass the pre-flight.`,
	DisableFlagParsing: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		env := otel.MergeEnv(otel.CurrentEnv(), otel.LocalEnv())

		if runPrintEnv {
			for _, kv := range otel.LocalEnv() {
				fmt.Println(kv)
			}
			return nil
		}

		if !runSkipReadyCheck {
			if err := preflightReady(cmd.Context()); err != nil {
				return err
			}
		}

		claudePath, err := otel.EnsureClaudeOnPath(exec.LookPath)
		if err != nil {
			return err
		}

		argv := append([]string{"claude"}, args...)
		return syscall.Exec(claudePath, argv, env)
	},
}

func preflightReady(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	ok, _, err := alloy.CheckReady(ctx)
	if ok {
		return nil
	}
	detail := ""
	if err != nil {
		detail = ": " + err.Error()
	}
	fmt.Fprintf(os.Stderr, "warning: local Alloy collector not ready at %s%s\n", alloy.ReadyURL, detail)
	fmt.Fprintf(os.Stderr, "  hint: run `spawn-claude collector install` (first time) or `spawn-claude collector restart`\n")
	fmt.Fprintf(os.Stderr, "  hint: pass --skip-ready-check to suppress this check\n")
	return fmt.Errorf("local collector not ready")
}

func init() {
	runCmd.Flags().BoolVar(&runSkipReadyCheck, "skip-ready-check", false, "skip the /-/ready preflight and exec claude regardless")
	runCmd.Flags().BoolVar(&runPrintEnv, "print-env", false, "print the OTEL env vars that would be exported, then exit")
	rootCmd.AddCommand(runCmd)
}
