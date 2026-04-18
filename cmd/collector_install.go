package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
	"github.com/hionnode/spawn-claude/internal/assets"
)

var installAlloyVersion string

var collectorInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download Alloy, install the system-wide LaunchDaemon, and start it",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCollectorInstall(cmd.Context(), installAlloyVersion)
	},
}

func init() {
	collectorInstallCmd.Flags().StringVar(&installAlloyVersion, "alloy-version", alloy.DefaultVersion, "Alloy release tag to install")
	collectorCmd.AddCommand(collectorInstallCmd)
}

func runCollectorInstall(ctx context.Context, version string) error {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("darwin/arm64 only (detected %s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	paths, err := alloy.ResolvePaths()
	if err != nil {
		return err
	}

	if err := removeLegacyPerUserAgent(ctx); err != nil {
		return err
	}

	if err := alloy.EnsureBinary(ctx, paths, version); err != nil {
		return err
	}
	slog.Debug("alloy binary ready", "path", paths.BinPath, "version", version)

	baseStage, configAction, err := stageBaseConfigIfNeeded(paths.ConfigFile)
	if err != nil {
		return err
	}
	if baseStage != "" {
		defer os.Remove(baseStage)
	}

	plistStage, err := stagePlist()
	if err != nil {
		return err
	}
	defer os.Remove(plistStage)

	script := buildInstallScript(paths, plistStage, baseStage, configAction)
	if err := alloy.SudoShell(ctx, script); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "waiting for %s\n", alloy.ReadyURL)
	if err := alloy.WaitReady(ctx, 10*time.Second); err != nil {
		printStderrTail(paths.StderrLog, 20)
		return err
	}
	fmt.Fprintf(os.Stdout, "READY (%s)\n", alloy.UIBase)
	return nil
}

// configAction describes what to do with the base config during install:
//   - "write" — no existing config, install the embedded base preset
//   - "replace" — existing config looks like the pre-Go bash placeholder; back
//     it up and replace with the base preset
//   - "keep" — existing real config, don't touch it
type configAction string

const (
	configWrite   configAction = "write"
	configReplace configAction = "replace"
	configKeep    configAction = "keep"
)

// stageBaseConfigIfNeeded writes the embedded base preset to a tempfile IF we
// determine the live /etc/alloy/config.alloy needs to be created or replaced.
// Returns the staged path (empty when no write needed) and the action.
func stageBaseConfigIfNeeded(configPath string) (string, configAction, error) {
	existing, err := os.ReadFile(configPath)
	switch {
	case err == nil:
		if !looksLikeBashEraPlaceholder(existing) {
			slog.Debug("config already exists, preserving", "path", configPath)
			return "", configKeep, nil
		}
	case !os.IsNotExist(err):
		return "", "", fmt.Errorf("read %s: %w", configPath, err)
	}

	base, err := assets.FS.ReadFile("presets/_base.alloy")
	if err != nil {
		return "", "", fmt.Errorf("read embedded base preset: %w", err)
	}

	stage, err := writeTempFile("spawn-claude-base-*.alloy", base)
	if err != nil {
		return "", "", err
	}
	if len(existing) == 0 {
		return stage, configWrite, nil
	}
	return stage, configReplace, nil
}

// looksLikeBashEraPlaceholder detects the pre-Go install.sh placeholder config
// that contained only comments and no actual Alloy declarations. Any real
// Alloy config declares at least one `otelcol.*` block.
func looksLikeBashEraPlaceholder(data []byte) bool {
	if len(data) >= 500 {
		return false
	}
	return !bytes.Contains(data, []byte("otelcol."))
}

func stagePlist() (string, error) {
	raw, err := assets.FS.ReadFile("platform/com.grafana.alloy.plist")
	if err != nil {
		return "", fmt.Errorf("read embedded plist: %w", err)
	}
	return writeTempFile("spawn-claude-plist-*.plist", raw)
}

func writeTempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// buildInstallScript assembles the single sudo shell script that performs
// every privileged install step: system dirs, plist install + lint, optional
// config write/replace, bootout of any existing daemon, bootstrap of the new
// one. Chaining with && means the first failure halts the rest.
func buildInstallScript(paths alloy.Paths, plistStage, baseStage string, action configAction) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "set -e\n")
	fmt.Fprintf(&b, "mkdir -p %q %q %q\n", paths.ConfigDir, paths.DataDir, paths.LogDir)
	fmt.Fprintf(&b, "install -o root -g wheel -m 0644 %q %q\n", plistStage, paths.AgentPath)
	fmt.Fprintf(&b, "plutil -lint %q >/dev/null\n", paths.AgentPath)

	switch action {
	case configWrite:
		fmt.Fprintf(&b, "install -o root -g wheel -m 0644 %q %q\n", baseStage, paths.ConfigFile)
	case configReplace:
		bak := paths.ConfigFile + ".bak"
		fmt.Fprintf(&b, "mv %q %q\n", paths.ConfigFile, bak)
		fmt.Fprintf(&b, "install -o root -g wheel -m 0644 %q %q\n", baseStage, paths.ConfigFile)
		fmt.Fprintf(&b, "echo 'replaced stale bash-era placeholder (backed up to %s)'\n", bak)
	}

	fmt.Fprintf(&b, "launchctl bootout system/%s 2>/dev/null || true\n", alloy.Label)
	fmt.Fprintf(&b, "launchctl bootstrap system %q\n", paths.AgentPath)
	return b.String()
}

// removeLegacyPerUserAgent cleans up the old per-user LaunchAgent plist at
// ~/Library/LaunchAgents/com.grafana.alloy.plist so it doesn't fight the new
// LaunchDaemon for 127.0.0.1:12345. Runs as the invoking user — no sudo
// needed because the file lives in $HOME.
func removeLegacyPerUserAgent(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // best-effort only
	}
	legacy := filepath.Join(home, "Library", "LaunchAgents", alloy.Label+".plist")
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}
	// Per-user bootout doesn't need sudo and safely no-ops if not loaded.
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), alloy.Label)
	_ = exec.CommandContext(ctx, "launchctl", "bootout", target).Run()
	if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove legacy %s: %w", legacy, err)
	}
	fmt.Fprintf(os.Stdout, "removed legacy per-user LaunchAgent at %s\n", legacy)
	return nil
}

func printStderrTail(path string, n int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	lines := make([]string, 0, n)
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1024*1024), 1024*1024)
	for s.Scan() {
		if len(lines) == n {
			lines = lines[1:]
		}
		lines = append(lines, s.Text())
	}
	fmt.Fprintf(os.Stderr, "--- last %d lines of %s ---\n", len(lines), path)
	for _, l := range lines {
		fmt.Fprintln(os.Stderr, l)
	}
}
