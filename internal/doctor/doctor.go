package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hionnode/spawn-claude/internal/alloy"
)

type Result struct {
	Name   string
	OK     bool
	Detail string
	Hint   string
}

func (r Result) String() string {
	marker := "fail"
	if r.OK {
		marker = " ok "
	}
	out := fmt.Sprintf("[%s] %-32s %s", marker, r.Name, r.Detail)
	if !r.OK && r.Hint != "" {
		out += "\n       hint: " + r.Hint
	}
	return out
}

// Run executes every check and returns the results in order.
func Run(ctx context.Context) []Result {
	paths, err := alloy.ResolvePaths()
	if err != nil {
		return []Result{{Name: "paths", OK: false, Detail: err.Error()}}
	}
	return []Result{
		checkClaudeOnPath(),
		checkAlloyBinary(paths),
		checkConfigFile(paths),
		checkLaunchDaemon(ctx),
		checkReady(ctx),
		checkOTLPPort(ctx, "OTLP HTTP receiver (:4318)", "127.0.0.1:4318"),
		checkOTLPPort(ctx, "OTLP gRPC receiver (:4317)", "127.0.0.1:4317"),
		checkMetricsEndpoint(ctx),
		checkClaudeProcessesHaveTelemetry(),
	}
}

func checkClaudeOnPath() Result {
	p, err := exec.LookPath("claude")
	if err != nil {
		return Result{
			Name:   "claude on PATH",
			Detail: err.Error(),
			Hint:   "install Claude Code (https://claude.ai/code) and put its binary on PATH",
		}
	}
	return Result{Name: "claude on PATH", OK: true, Detail: p}
}

func checkAlloyBinary(paths alloy.Paths) Result {
	info, err := os.Stat(paths.BinPath)
	if err != nil {
		return Result{
			Name:   "alloy binary installed",
			Detail: err.Error(),
			Hint:   "run `spawn-claude collector install`",
		}
	}
	if info.Mode()&0o111 == 0 {
		return Result{
			Name:   "alloy binary installed",
			Detail: fmt.Sprintf("%s exists but is not executable", paths.BinPath),
			Hint:   "chmod +x " + paths.BinPath,
		}
	}
	return Result{Name: "alloy binary installed", OK: true, Detail: paths.BinPath}
}

func checkConfigFile(paths alloy.Paths) Result {
	info, err := os.Stat(paths.ConfigFile)
	if err != nil {
		return Result{
			Name:   "collector config present",
			Detail: err.Error(),
			Hint:   "run `spawn-claude collector install` or `spawn-claude collector configure local-debug`",
		}
	}
	return Result{Name: "collector config present", OK: true, Detail: fmt.Sprintf("%s (%d bytes)", paths.ConfigFile, info.Size())}
}

func checkLaunchDaemon(ctx context.Context) Result {
	s, err := alloy.Status(ctx)
	if err != nil {
		return Result{Name: "LaunchDaemon loaded", Detail: err.Error(), Hint: "run `spawn-claude collector install`"}
	}
	if !s.Loaded {
		return Result{
			Name:   "LaunchDaemon loaded",
			Detail: "launchctl doesn't know about " + alloy.Label,
			Hint:   "run `spawn-claude collector install`",
		}
	}
	if !s.Running {
		return Result{
			Name:   "LaunchDaemon loaded",
			Detail: fmt.Sprintf("loaded but not running (last exit %d)", s.LastExit),
			Hint:   "run `spawn-claude collector restart` and check `spawn-claude collector logs`",
		}
	}
	return Result{Name: "LaunchDaemon loaded", OK: true, Detail: fmt.Sprintf("PID %d, last exit %d", s.PID, s.LastExit)}
}

func checkReady(parent context.Context) Result {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	ok, code, err := alloy.CheckReady(ctx)
	if ok {
		return Result{Name: "UI /-/ready", OK: true, Detail: fmt.Sprintf("%s → 200", alloy.ReadyURL)}
	}
	detail := fmt.Sprintf("%s", alloy.ReadyURL)
	if err != nil {
		detail = fmt.Sprintf("%s: %v", alloy.ReadyURL, err)
	} else {
		detail = fmt.Sprintf("%s → %d", alloy.ReadyURL, code)
	}
	return Result{Name: "UI /-/ready", Detail: detail, Hint: "run `spawn-claude collector restart`"}
}

func checkOTLPPort(ctx context.Context, name, addr string) Result {
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	d := net.Dialer{}
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return Result{
			Name:   name,
			Detail: err.Error(),
			Hint:   "the running config is missing an `otelcol.receiver.otlp` block; try `spawn-claude collector configure local-debug`",
		}
	}
	conn.Close()
	return Result{Name: name, OK: true, Detail: addr + " accepting connections"}
}

func checkMetricsEndpoint(parent context.Context) Result {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, alloy.UIBase+"/metrics", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Result{Name: "Alloy /metrics reachable", Detail: err.Error(), Hint: "run `spawn-claude collector restart`"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{Name: "Alloy /metrics reachable", Detail: fmt.Sprintf("/metrics → %d", resp.StatusCode)}
	}
	// Drain body so the connection can be reused, but we don't need the content.
	_, _ = io.Copy(io.Discard, resp.Body)
	return Result{Name: "Alloy /metrics reachable", OK: true, Detail: alloy.UIBase + "/metrics"}
}

// checkClaudeProcessesHaveTelemetry enumerates the user's running `claude`
// processes and flags any that lack CLAUDE_CODE_ENABLE_TELEMETRY=1 in their
// environment. A session started before `spawn-claude run` (or without the
// telemetry env wired in) will emit nothing to the collector — which looks
// identical to a broken pipeline from Grafana.
func checkClaudeProcessesHaveTelemetry() Result {
	name := "claude processes emit telemetry"
	pids, err := findClaudePIDs()
	if err != nil {
		return Result{Name: name, Detail: "pgrep failed: " + err.Error()}
	}
	if len(pids) == 0 {
		return Result{Name: name, OK: true, Detail: "no running claude processes"}
	}
	var silent []int
	for _, pid := range pids {
		env, err := readProcessEnv(pid)
		if err != nil {
			continue
		}
		if !strings.Contains(env, "CLAUDE_CODE_ENABLE_TELEMETRY=1") {
			silent = append(silent, pid)
		}
	}
	if len(silent) == 0 {
		return Result{Name: name, OK: true, Detail: fmt.Sprintf("%d process(es), all have CLAUDE_CODE_ENABLE_TELEMETRY=1", len(pids))}
	}
	strs := make([]string, len(silent))
	for i, p := range silent {
		strs[i] = strconv.Itoa(p)
	}
	return Result{
		Name:   name,
		Detail: fmt.Sprintf("%d of %d process(es) missing CLAUDE_CODE_ENABLE_TELEMETRY=1 (pids %s)", len(silent), len(pids), strings.Join(strs, ", ")),
		Hint:   "those sessions were started without telemetry env and emit nothing; relaunch them via `spawn-claude run`",
	}
}

func findClaudePIDs() ([]int, error) {
	cmd := exec.Command("pgrep", "-u", strconv.Itoa(os.Getuid()), "-x", "claude")
	out, err := cmd.Output()
	if err != nil {
		// pgrep exits 1 when there are no matches — treat as "none found".
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func readProcessEnv(pid int) (string, error) {
	// macOS `ps -E` prints env vars inline with the command; -ww disables
	// truncation, -o command= suppresses the header.
	out, err := exec.Command("ps", "-E", "-ww", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	return string(out), err
}

// AnyFailed reports whether any result in rs has OK=false.
func AnyFailed(rs []Result) bool {
	for _, r := range rs {
		if !r.OK {
			return true
		}
	}
	return false
}
