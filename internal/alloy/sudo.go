package alloy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// SudoShell runs the given /bin/sh script under sudo with stdin/stdout/stderr
// attached to the current process, so the user sees any password prompt
// inline. All privileged steps (chown/install/launchctl in the system domain)
// funnel through here; bundling them into a single script minimizes prompts
// because sudo caches credentials between invocations in a single session.
func SudoShell(ctx context.Context, script string) error {
	cmd := exec.CommandContext(ctx, "sudo", "/bin/sh", "-c", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo script failed: %w", err)
	}
	return nil
}
