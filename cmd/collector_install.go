package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hionnode/spawn-claude/internal/alloy"
	"github.com/hionnode/spawn-claude/internal/assets"
)

var installAlloyVersion string

var collectorInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download Alloy, render the LaunchAgent, and start it",
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
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	if err := alloy.EnsureBinary(ctx, paths, version); err != nil {
		return err
	}
	slog.Debug("alloy binary ready", "path", paths.BinPath, "version", version)

	if err := writeConfigIfAbsent(paths.ConfigFile); err != nil {
		return err
	}

	if err := renderPlist(paths); err != nil {
		return err
	}

	if err := alloy.PlutilLint(ctx, paths.AgentPath); err != nil {
		return err
	}

	_ = alloy.Bootout(ctx, paths.AgentPath)
	if err := alloy.Bootstrap(ctx, paths.AgentPath); err != nil {
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

func writeConfigIfAbsent(path string) error {
	if _, err := os.Stat(path); err == nil {
		slog.Debug("config already exists, preserving", "path", path)
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	data, err := assets.FS.ReadFile("presets/_base.alloy")
	if err != nil {
		return fmt.Errorf("read embedded base preset: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	slog.Debug("installed placeholder config", "path", path)
	return nil
}

func renderPlist(paths alloy.Paths) error {
	raw, err := assets.FS.ReadFile("platform/com.grafana.alloy.plist")
	if err != nil {
		return fmt.Errorf("read embedded plist: %w", err)
	}
	rendered := strings.ReplaceAll(string(raw), "__HOME__", paths.Home)
	if err := os.WriteFile(paths.AgentPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", paths.AgentPath, err)
	}
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
