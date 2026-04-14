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
	"github.com/hionnode/spawn-claude/internal/config"
	"github.com/hionnode/spawn-claude/internal/otel"
	"github.com/hionnode/spawn-claude/internal/presets"
	"github.com/hionnode/spawn-claude/internal/secrets"
)

var (
	runSkipReadyCheck bool
	runPrintEnv       bool
	runDirect         bool
	runVendorOverride string
)

var runCmd = &cobra.Command{
	Use:   "run [-- claude args...]",
	Short: "Run `claude` with OTLP telemetry (via the local collector, or direct to a vendor with --direct)",
	Long: `run executes the ` + "`claude`" + ` binary with OTLP env vars so Claude Code emits
telemetry. Default target is the local Alloy collector on
http://127.0.0.1:4318 (protocol http/protobuf). Any arguments after ` + "`--`" + `
are forwarded verbatim to claude.

Use --direct to bypass the local collector and send telemetry straight to
the vendor configured in ~/.config/spawn-claude/config.toml (or the preset
named by --vendor). Required secrets are loaded from
~/.config/spawn-claude/secrets.env.`,
	DisableFlagParsing: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		pairs, err := buildRunEnv(runDirect, runVendorOverride)
		if err != nil {
			return err
		}

		if runPrintEnv {
			for _, kv := range pairs {
				fmt.Println(kv)
			}
			return nil
		}

		if !runDirect && !runSkipReadyCheck {
			if err := preflightReady(cmd.Context()); err != nil {
				return err
			}
		}

		claudePath, err := otel.EnsureClaudeOnPath(exec.LookPath)
		if err != nil {
			return err
		}

		env := otel.MergeEnv(otel.CurrentEnv(), pairs)
		argv := append([]string{"claude"}, args...)
		return syscall.Exec(claudePath, argv, env)
	},
}

func buildRunEnv(direct bool, vendorOverride string) ([]string, error) {
	if !direct {
		return otel.LocalEnv(), nil
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths.File)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	vendor := vendorOverride
	if vendor == "" {
		vendor = cfg.Direct.Vendor
	}
	if vendor == "" {
		return nil, fmt.Errorf("--direct requires [direct].vendor in %s or --vendor=<name>", paths.File)
	}

	preset, err := presets.Get(vendor)
	if err != nil {
		return nil, err
	}
	if preset.DirectEnv == nil {
		return nil, fmt.Errorf("preset %q cannot be used in --direct mode (collector-only preset)", preset.Name)
	}

	secrets.EnsureSecureMode(paths.Secrets, func(msg string) {
		fmt.Fprintln(os.Stderr, "warning:", msg)
	})
	loaded, err := secrets.Load(paths.Secrets)
	if err != nil {
		return nil, err
	}
	return preset.DirectEnv(loaded)
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
	runCmd.Flags().BoolVar(&runDirect, "direct", false, "bypass the local collector and send OTLP straight to the configured vendor")
	runCmd.Flags().StringVar(&runVendorOverride, "vendor", "", "override [direct].vendor from config.toml (only meaningful with --direct)")
	rootCmd.AddCommand(runCmd)
}
