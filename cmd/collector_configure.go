package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
	"github.com/hionnode/spawn-claude/internal/config"
	"github.com/hionnode/spawn-claude/internal/presets"
	"github.com/hionnode/spawn-claude/internal/secrets"
)

var (
	configureList          bool
	configureSkipReload    bool
	configureSkipValidate  bool
	configureSetVendorMode bool
)

var collectorConfigureCmd = &cobra.Command{
	Use:   "configure [preset]",
	Short: "Render a preset (signoz-cloud | grafana-cloud | local-debug) into ~/.config/alloy/config.alloy",
	Long: `configure writes ~/.config/alloy/config.alloy from one of the built-in
presets, substituting required secrets from ~/.config/spawn-claude/secrets.env.

Use --list to see every available preset. Use --set-direct to also update
[direct].vendor in ~/.config/spawn-claude/config.toml so that
` + "`spawn-claude run --direct`" + ` defaults to the same vendor.

The existing config.alloy is backed up to config.alloy.bak before the
rename. After writing, the config is validated via ` + "`alloy fmt`" + ` and then
hot-reloaded via POST /-/reload (both can be skipped).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if configureList {
			printPresetList()
			return nil
		}
		if len(args) == 0 {
			return fmt.Errorf("preset name required (see --list)")
		}
		return runConfigure(cmd.Context(), args[0])
	},
}

func printPresetList() {
	fmt.Printf("%-14s  %s\n", "NAME", "DESCRIPTION")
	for _, p := range presets.All() {
		fmt.Printf("%-14s  %s\n", p.Name, p.Description)
		if len(p.RequiredSecrets) > 0 {
			fmt.Printf("%-14s  required secrets: %s\n", "", strings.Join(p.RequiredSecrets, ", "))
		}
	}
}

func runConfigure(ctx context.Context, name string) error {
	preset, err := presets.Get(name)
	if err != nil {
		return err
	}

	alloyPaths, err := alloy.ResolvePaths()
	if err != nil {
		return err
	}
	cfgPaths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	secrets.EnsureSecureMode(cfgPaths.Secrets, func(msg string) {
		fmt.Fprintln(os.Stderr, "warning:", msg)
	})
	loaded, err := secrets.Load(cfgPaths.Secrets)
	if err != nil {
		return err
	}

	rendered, err := presets.Render(preset, loaded)
	if err != nil {
		return err
	}

	stagePath := alloyPaths.ConfigFile + ".new"
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(stagePath, rendered, 0o644); err != nil {
		return err
	}

	if !configureSkipValidate {
		if err := validateAlloyConfig(ctx, alloyPaths.BinPath, stagePath); err != nil {
			_ = os.Remove(stagePath)
			return err
		}
	}

	if _, err := os.Stat(alloyPaths.ConfigFile); err == nil {
		bak := alloyPaths.ConfigFile + ".bak"
		if err := os.Rename(alloyPaths.ConfigFile, bak); err != nil {
			return fmt.Errorf("backup existing config to %s: %w", bak, err)
		}
	}
	if err := os.Rename(stagePath, alloyPaths.ConfigFile); err != nil {
		return fmt.Errorf("move %s into place: %w", stagePath, err)
	}
	fmt.Printf("wrote %s (preset %q)\n", alloyPaths.ConfigFile, preset.Name)

	if configureSetVendorMode && preset.DirectEnv != nil {
		cfg, err := config.Load(cfgPaths.File)
		if err != nil {
			return err
		}
		cfg.Direct.Vendor = preset.Name
		if err := config.Save(cfgPaths.File, cfg); err != nil {
			return err
		}
		fmt.Printf("updated [direct].vendor = %q in %s\n", preset.Name, cfgPaths.File)
	}

	if configureSkipReload {
		return nil
	}
	if err := reloadCollector(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: hot reload failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "  hint: try `spawn-claude collector restart`\n")
		return nil
	}
	fmt.Println("reloaded")
	return nil
}

func validateAlloyConfig(ctx context.Context, alloyBin, path string) error {
	if _, err := os.Stat(alloyBin); err != nil {
		// Alloy not installed yet — skip validation with a notice.
		fmt.Fprintf(os.Stderr, "note: %s not found; skipping `alloy fmt` validation\n", alloyBin)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, alloyBin, "fmt", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("alloy fmt %s failed: %s: %w", path, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func init() {
	collectorConfigureCmd.Flags().BoolVar(&configureList, "list", false, "list available presets and exit")
	collectorConfigureCmd.Flags().BoolVar(&configureSkipReload, "skip-reload", false, "don't POST /-/reload after writing")
	collectorConfigureCmd.Flags().BoolVar(&configureSkipValidate, "skip-validate", false, "don't run `alloy fmt` on the rendered config")
	collectorConfigureCmd.Flags().BoolVar(&configureSetVendorMode, "set-direct", false, "also set [direct].vendor in ~/.config/spawn-claude/config.toml to this preset")
	collectorCmd.AddCommand(collectorConfigureCmd)
}
