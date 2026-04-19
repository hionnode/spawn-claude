package otel

import (
	"fmt"
	"os"
)

// LocalEndpoint is the OTLP HTTP endpoint for the spawn-claude-managed Alloy
// collector. It matches the receiver block in the default _base.alloy preset.
const LocalEndpoint = "http://127.0.0.1:4318"

// LocalProtocol is the OTLP protocol used when sending to the local collector.
// We prefer http/protobuf over grpc because it works without a gRPC client on
// the Claude Code side and keeps the endpoint URL scheme obvious.
const LocalProtocol = "http/protobuf"

// LocalEnv returns the environment variable pairs that configure Claude Code
// to emit OTLP telemetry to the local Alloy collector. Order matters for the
// `spawn-claude run --print-env` output but not for exec.
func LocalEnv() []string {
	return []string{
		"CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"OTEL_METRICS_EXPORTER=otlp",
		"OTEL_LOGS_EXPORTER=otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL=" + LocalProtocol,
		"OTEL_EXPORTER_OTLP_ENDPOINT=" + LocalEndpoint,
		"OTEL_METRIC_EXPORT_INTERVAL=10000",
		"OTEL_LOGS_EXPORT_INTERVAL=5000",
		// Claude's OTel SDK hardcodes Delta temporality for monotonic sums
		// and currently ignores this env var. We set it anyway as
		// belt-and-suspenders in case a future Claude version honors it.
		// The load-bearing fix is an `otelcol.processor.deltatocumulative`
		// in the Alloy pipeline whenever metrics route through
		// `otelcol.exporter.prometheus` → `prometheus.remote_write`; see
		// GRAFANA-CLOUD.md § "Delta temporality silently drops metrics".
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative",
	}
}

// MergeEnv returns a copy of the current environment with the given pairs
// applied last, so they override any existing values the user already set.
func MergeEnv(base []string, overrides []string) []string {
	index := make(map[string]int, len(base))
	merged := append([]string(nil), base...)
	for i, kv := range base {
		if k := keyOf(kv); k != "" {
			index[k] = i
		}
	}
	for _, kv := range overrides {
		k := keyOf(kv)
		if k == "" {
			continue
		}
		if i, ok := index[k]; ok {
			merged[i] = kv
		} else {
			index[k] = len(merged)
			merged = append(merged, kv)
		}
	}
	return merged
}

func keyOf(kv string) string {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i]
		}
	}
	return ""
}

// CurrentEnv is a thin wrapper around os.Environ kept here so callers don't
// have to reach into `os` just to build a base slice for MergeEnv.
func CurrentEnv() []string { return os.Environ() }

// EnsureClaudeOnPath resolves the `claude` binary and returns its absolute
// path. Failure messages suggest the standard install paths.
func EnsureClaudeOnPath(lookPath func(string) (string, error)) (string, error) {
	p, err := lookPath("claude")
	if err != nil {
		return "", fmt.Errorf("`claude` not found on PATH: %w\n  hint: install Claude Code (https://claude.ai/code) and ensure its binary is on PATH", err)
	}
	return p, nil
}
