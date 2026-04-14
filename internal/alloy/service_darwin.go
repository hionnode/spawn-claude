//go:build darwin

package alloy

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
)

// domain returns the per-user gui launchd domain: "gui/<uid>".
func domain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// Bootout unloads the LaunchAgent for this user. Failure is non-fatal because
// the agent may simply not be loaded yet (e.g., first install).
func Bootout(ctx context.Context, plistPath string) error {
	cmd := exec.CommandContext(ctx, "launchctl", "bootout", domain(), plistPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("launchctl bootout failed (expected on first install)",
			"plist", plistPath, "output", string(bytes.TrimSpace(out)), "err", err)
	}
	return nil
}

// Bootstrap loads the LaunchAgent for this user.
func Bootstrap(ctx context.Context, plistPath string) error {
	cmd := exec.CommandContext(ctx, "launchctl", "bootstrap", domain(), plistPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap %s %s: %s: %w", domain(), plistPath, bytes.TrimSpace(out), err)
	}
	return nil
}

// Kickstart hard-restarts the running service.
func Kickstart(ctx context.Context) error {
	target := fmt.Sprintf("%s/%s", domain(), Label)
	cmd := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl kickstart -k %s: %s: %w", target, bytes.TrimSpace(out), err)
	}
	return nil
}

// PlutilLint validates a plist file by shelling out to plutil. Mirrors install.sh L51.
func PlutilLint(ctx context.Context, plistPath string) error {
	cmd := exec.CommandContext(ctx, "plutil", "-lint", plistPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("plutil -lint %s: %s: %w", plistPath, bytes.TrimSpace(out), err)
	}
	return nil
}

// ServiceStatus is the parsed output of `launchctl list <label>`.
type ServiceStatus struct {
	Loaded   bool
	PID      int  // 0 if not running
	LastExit int  // last exit status; 0 if healthy
	Running  bool // true iff PID != 0
}

var (
	pidRE  = regexp.MustCompile(`"PID"\s*=\s*(\d+);`)
	exitRE = regexp.MustCompile(`"LastExitStatus"\s*=\s*(-?\d+);`)
)

// Status queries launchctl for the current state of the agent.
func Status(ctx context.Context) (ServiceStatus, error) {
	cmd := exec.CommandContext(ctx, "launchctl", "list", Label)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 113 / "Could not find service" means not loaded.
		return ServiceStatus{Loaded: false}, nil
	}
	s := ServiceStatus{Loaded: true}
	if m := pidRE.FindStringSubmatch(string(out)); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			s.PID = v
			s.Running = v > 0
		}
	}
	if m := exitRE.FindStringSubmatch(string(out)); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			s.LastExit = v
		}
	}
	return s, nil
}
