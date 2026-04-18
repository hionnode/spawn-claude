//go:build darwin

package alloy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
)

// systemDomain is the launchctl domain for system-wide LaunchDaemons. Moving
// from per-user LaunchAgent (gui/<uid>) to LaunchDaemon (system) means every
// bootstrap/bootout/kickstart call needs sudo.
const systemDomain = "system"

func serviceTarget() string {
	return systemDomain + "/" + Label
}

// Bootout unloads the LaunchDaemon. Failure is non-fatal because the daemon
// may simply not be loaded (first install, or after a previous uninstall).
func Bootout(ctx context.Context, plistPath string) error {
	script := fmt.Sprintf("launchctl bootout %s %q 2>&1 || true", systemDomain, plistPath)
	if err := SudoShell(ctx, script); err != nil {
		slog.Debug("launchctl bootout failed (expected on first install)", "plist", plistPath, "err", err)
	}
	return nil
}

// Bootstrap loads the LaunchDaemon. Requires sudo.
func Bootstrap(ctx context.Context, plistPath string) error {
	script := fmt.Sprintf("launchctl bootstrap %s %q", systemDomain, plistPath)
	return SudoShell(ctx, script)
}

// Kickstart hard-restarts the running daemon. Requires sudo.
func Kickstart(ctx context.Context) error {
	script := fmt.Sprintf("launchctl kickstart -k %s", serviceTarget())
	return SudoShell(ctx, script)
}

// PlutilLint validates a plist file by shelling out to plutil. Runs without
// sudo (plutil only reads the file).
func PlutilLint(ctx context.Context, plistPath string) error {
	cmd := exec.CommandContext(ctx, "plutil", "-lint", plistPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("plutil -lint %s: %s: %w", plistPath, bytes.TrimSpace(out), err)
	}
	return nil
}

// ServiceStatus is the parsed output of `launchctl print system/<label>`.
type ServiceStatus struct {
	Loaded   bool
	PID      int  // 0 if not running
	LastExit int  // last exit status; 0 if healthy
	Running  bool // true iff PID != 0
}

var (
	printPIDRE  = regexp.MustCompile(`(?m)^\s*pid\s*=\s*(\d+)`)
	printExitRE = regexp.MustCompile(`(?m)^\s*last exit code\s*=\s*(-?\d+)`)
)

// Status queries launchctl for the current state of the daemon. `launchctl
// print` on a system-domain target works without sudo for reading, so we
// don't need to prompt on status checks.
func Status(ctx context.Context) (ServiceStatus, error) {
	cmd := exec.CommandContext(ctx, "launchctl", "print", serviceTarget())
	out, err := cmd.CombinedOutput()
	if err != nil {
		// "Could not find service" / exit 113 means not loaded.
		return ServiceStatus{Loaded: false}, nil
	}
	s := ServiceStatus{Loaded: true}
	if m := printPIDRE.FindStringSubmatch(string(out)); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			s.PID = v
			s.Running = v > 0
		}
	}
	if m := printExitRE.FindStringSubmatch(string(out)); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			s.LastExit = v
		}
	}
	return s, nil
}
