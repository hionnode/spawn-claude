# spawn-claude

A Go CLI for macOS that manages a per-user [Grafana Alloy](https://grafana.com/docs/alloy/) collector for Claude Code OTLP telemetry. The long-term goal is a single `spawn-claude` command that both (a) runs `claude` with the OTLP environment variables from the [SignOz Claude Code monitoring guide](https://signoz.io/docs/claude-code-monitoring/) and (b) forwards that telemetry through a local Alloy collector that can be pointed at SignOz, Grafana Cloud, or any other OTLP-compatible backend.

## Status

Pre-1.0. The **collector lifecycle** commands and the **`spawn-claude run`** wrapper are in. Preset-based backend configuration (`configure <vendor>`) lands next — see [Roadmap](#roadmap).

Platform: **macOS on Apple Silicon (darwin/arm64) only.**

## Install

```bash
go install github.com/hionnode/spawn-claude@latest
```

Signed binary releases and a Homebrew tap land with PR6.

## Quickstart

```bash
spawn-claude collector install                # one-time: downloads Alloy, starts the LaunchAgent
spawn-claude run                              # runs claude with OTLP telemetry pointed at the local collector
spawn-claude run -- --help                    # anything after -- is forwarded to claude
```

`install` fetches the pinned Alloy binary (`v1.15.1`), writes a default config at `~/.config/alloy/config.alloy` that receives OTLP on `:4317` (gRPC) and `:4318` (HTTP) and logs every payload to `~/Library/Logs/alloy/stderr.log`, then loads a LaunchAgent that keeps the collector alive across reboots.

`run` exec's `claude` with these environment variables set:

```
CLAUDE_CODE_ENABLE_TELEMETRY=1
OTEL_METRICS_EXPORTER=otlp
OTEL_LOGS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
OTEL_METRIC_EXPORT_INTERVAL=10000
OTEL_LOGS_EXPORT_INTERVAL=5000
```

Confirm ingestion by tailing `spawn-claude collector logs -f` while claude is running. To forward the telemetry to a real backend (SignOz Cloud, Grafana Cloud, …), wait for the preset system in PR3 or hand-edit `~/.config/alloy/config.alloy` and `spawn-claude collector reload`.

## Commands

| Command | What it does |
|---|---|
| `spawn-claude collector install [--alloy-version vX.Y.Z]` | Download Alloy, render the plist, `launchctl bootstrap` it, wait for ready. Idempotent. |
| `spawn-claude collector uninstall [--purge]` | `launchctl bootout` and remove the plist. `--purge` also deletes the binary, config, and logs. |
| `spawn-claude collector status` | Show whether the agent is loaded + whether the UI is ready. Exits 1 on any failure. |
| `spawn-claude collector restart` | `launchctl kickstart -k`, then wait for ready. |
| `spawn-claude collector reload` | Hot-reload the config via `POST /-/reload` (no process restart). |
| `spawn-claude collector logs [-f]` | Print `~/Library/Logs/alloy/stderr.log`. `-f` follows. |
| `spawn-claude run [-- claude-args]` | Exec `claude` with OTLP env vars set to the local collector. Supports `--skip-ready-check` and `--print-env`. |
| `spawn-claude version` | Print the spawn-claude version and the default Alloy version. |

## File layout after install

| What | Path |
|---|---|
| Binary | `~/.local/bin/alloy` |
| Config | `~/.config/alloy/config.alloy` |
| Data / WAL | `~/.config/alloy/data/` |
| Logs | `~/Library/Logs/alloy/{stdout,stderr}.log` |
| LaunchAgent | `~/Library/LaunchAgents/com.grafana.alloy.plist` |
| UI | http://127.0.0.1:12345 |

## Why download a prebuilt binary instead of `brew install grafana/grafana/alloy`

The Homebrew formula builds from source. On macOS 14.5 with Command Line Tools 15.3, `go build` fails with:

```
ld: B/BL out of range -148443508 (max +/-128MB) from ...
clang: error: linker command failed with exit code 1
```

Apple's old `ld` in CLT 15.3 can't resolve branch islands for Alloy's ~500 MB Go binary on arm64. Newer CLT (16.x with `ld-prime`) fixes it, but upgrading CLT needs a ~5 GB install and sudo. Tracking issue: [grafana/homebrew-grafana#134](https://github.com/grafana/homebrew-grafana/issues/134). `spawn-claude` sidesteps the problem by fetching the prebuilt `alloy-darwin-arm64.zip` directly from the upstream GitHub release.

## Uninstall

```bash
spawn-claude collector uninstall          # unload + remove LaunchAgent only
spawn-claude collector uninstall --purge  # also remove binary, ~/.config/alloy/, and logs
```

## Roadmap

- ~~**PR2** — `spawn-claude run`.~~ ✅ shipped
- **PR3** — Preset registry (`spawn-claude collector configure <signoz-cloud|grafana-cloud|...>`) with secret templating and `alloy fmt` validation. `run --direct` to bypass the local collector.
- **PR4** — `spawn-claude doctor` end-to-end health check, including a smoke-test trace export.
- **PR5** — goreleaser pipeline, signed macOS binaries, Homebrew tap.

## Development

```bash
go build -o spawn-claude .
go vet ./...
./spawn-claude --help
```

## License

MIT — see [LICENSE](LICENSE).
