# spawn-claude

A Go CLI for macOS that manages a system-wide [Grafana Alloy](https://grafana.com/docs/alloy/) collector for Claude Code OTLP telemetry. A single `spawn-claude` command both (a) runs `claude` with the OTLP environment variables from the [SignOz Claude Code monitoring guide](https://signoz.io/docs/claude-code-monitoring/) and (b) forwards that telemetry through a local Alloy collector that can be pointed at SignOz, Grafana Cloud, or any other OTLP-compatible backend.

Alloy installs at the standard system paths (`/etc/alloy/config.alloy`, `/usr/local/bin/alloy`, `/Library/LaunchDaemons/com.grafana.alloy.plist`) and runs as a root-owned LaunchDaemon — same layout Homebrew/`.deb`/`.rpm` would give you, so snippets from Grafana and SignOz onboarding UIs paste into the config unchanged. spawn-claude prompts for `sudo` inline whenever it writes those paths.

## Status

Pre-1.0 but feature-complete. Working: collector lifecycle, `spawn-claude run` (local + `--direct`), vendor presets (SignOz Cloud, Grafana Cloud), end-to-end `spawn-claude doctor`, and a goreleaser-driven release pipeline with a `curl | sh` bootstrap installer.

Platform: **macOS on Apple Silicon (darwin/arm64) only.**

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/hionnode/spawn-claude/main/install.sh | sh
```

This downloads the latest GitHub release tarball, verifies its sha256 against the published `checksums.txt`, and drops the binary at `$HOME/.local/bin/spawn-claude`. Override the location with `BIN_DIR=/usr/local/bin sh install.sh`. The release binary is **not Apple-notarized** — the install script strips the quarantine xattr so Gatekeeper doesn't block on first run; full notarization requires a paid Apple Developer account and is intentionally out of scope.

If you have Go 1.23+ and prefer to build from source:

```bash
go install github.com/hionnode/spawn-claude@latest
```

## Quickstart

```bash
spawn-claude collector install                # one-time: downloads Alloy, starts the LaunchDaemon (prompts for sudo)
spawn-claude run                              # runs claude with OTLP telemetry pointed at the local collector
spawn-claude run -- --help                    # anything after -- is forwarded to claude
```

`install` fetches the pinned Alloy binary (`v1.15.1`), installs it to `/usr/local/bin/alloy`, writes a default config at `/etc/alloy/config.alloy` that receives OTLP on `:4317` (gRPC) and `:4318` (HTTP) and logs every payload to `/var/log/alloy/stderr.log`, then loads a LaunchDaemon that keeps the collector alive across reboots. All privileged steps are bundled into a single `sudo` invocation so you're prompted once.

`run` exec's `claude` with these environment variables set:

```
CLAUDE_CODE_ENABLE_TELEMETRY=1
OTEL_METRICS_EXPORTER=otlp
OTEL_LOGS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
OTEL_METRIC_EXPORT_INTERVAL=10000
OTEL_LOGS_EXPORT_INTERVAL=5000
OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative
```

Confirm ingestion by tailing `spawn-claude collector logs -f` while claude is running.

For a walkthrough of what each command does under the hood — and the exact shell commands to do it manually without `spawn-claude` — see [MANUAL.md](MANUAL.md).

### Forwarding telemetry to a real backend

```bash
spawn-claude collector configure --list                    # see available presets
$EDITOR ~/.config/spawn-claude/secrets.env && chmod 600 $_ # drop ingestion creds
spawn-claude collector configure grafana-cloud             # or signoz-cloud — prompts for sudo
```

`configure` reads secrets (KEY=VALUE, shell-style) from `~/.config/spawn-claude/secrets.env`, renders the preset's Alloy config with them, validates via `alloy fmt`, atomically `install`s `/etc/alloy/config.alloy` (backing up the previous one to `.bak`) under `sudo`, and `POST /-/reload`s the running collector. The preset templates are embedded in the `spawn-claude` binary; see `internal/assets/presets/*.alloy` in-tree.

Vendor onboarding UIs that say "paste this block into `/etc/alloy/config.alloy`" work unchanged — the file they reference is the exact file `spawn-claude` manages.

If you arrived here via Grafana Cloud's onboarding flow (`install-macos-binary.sh`) and your existing config uses `sys.env("GCLOUD_RW_API_KEY")` for basic-auth, read [GRAFANA-CLOUD.md](GRAFANA-CLOUD.md) — there's a specific LaunchDaemon env-inheritance gotcha you'll hit on the first reboot.

Required secrets per preset:

| Preset | Required keys in `~/.config/spawn-claude/secrets.env` |
|---|---|
| `signoz-cloud` | `SIGNOZ_ENDPOINT` (e.g. `https://ingest.us.signoz.cloud:443`), `SIGNOZ_INGESTION_KEY` |
| `grafana-cloud` | `GRAFANA_CLOUD_OTLP_ENDPOINT`, `GRAFANA_CLOUD_OTLP_USERNAME`, `GRAFANA_CLOUD_OTLP_PASSWORD` |
| `local-debug` | (none — this is the default collector-only config) |

### `run --direct` (bypass the local collector)

```bash
spawn-claude collector configure signoz-cloud --set-direct   # also writes config.toml
spawn-claude run --direct -- <claude args>                   # sends OTLP straight to SignOz
```

`--direct` reads `[direct].vendor` from `~/.config/spawn-claude/config.toml` (override with `--vendor=<preset>`), loads the preset's required secrets, and exports the per-vendor OTLP env block before exec'ing `claude`. No local collector is involved.

## Commands

| Command | What it does |
|---|---|
| `spawn-claude collector install [--alloy-version vX.Y.Z]` | Download Alloy, install binary + plist, `launchctl bootstrap system`, wait for ready. Idempotent. Prompts for sudo. |
| `spawn-claude collector uninstall [--purge]` | `launchctl bootout system` + remove plist and binary. `--purge` also deletes `/etc/alloy`, `/var/lib/alloy`, and `/var/log/alloy`. Prompts for sudo. |
| `spawn-claude collector status` | Show whether the daemon is loaded + whether the UI is ready. Exits 1 on any failure. |
| `spawn-claude collector restart` | `launchctl kickstart -k system/com.grafana.alloy`, then wait for ready. Prompts for sudo. |
| `spawn-claude collector reload` | Hot-reload the config via `POST /-/reload` (no process restart, no sudo). |
| `spawn-claude collector logs [-f]` | Print `/var/log/alloy/stderr.log`. `-f` follows. |
| `spawn-claude collector configure <preset>` | Render a preset into `/etc/alloy/config.alloy`, `alloy fmt` it, sudo-install atomically, hot-reload. `--list` shows options. `--set-direct` also updates `config.toml`. |
| `spawn-claude run [-- claude-args]` | Exec `claude` with OTLP env vars pointed at the local collector. Flags: `--direct`, `--vendor`, `--skip-ready-check`, `--print-env`. |
| `spawn-claude doctor` | Run end-to-end health checks (claude on PATH, alloy installed, LaunchDaemon running, /-/ready, OTLP ports listening, claude processes have telemetry env). Exits 1 on any failure. |
| `spawn-claude version` | Print the spawn-claude version and the default Alloy version. |

## File layout after install

| What | Path | Owner |
|---|---|---|
| Binary | `/usr/local/bin/alloy` | root:wheel 0755 |
| Config | `/etc/alloy/config.alloy` | root:wheel 0644 |
| Data / WAL | `/var/lib/alloy/data/` | root |
| Logs | `/var/log/alloy/{stdout,stderr}.log` | root:wheel 0644 |
| LaunchDaemon | `/Library/LaunchDaemons/com.grafana.alloy.plist` | root:wheel 0644 |
| UI | http://127.0.0.1:12345 | |
| OTLP (gRPC / HTTP) | 127.0.0.1:4317 / 127.0.0.1:4318 | |

Config and logs are world-readable, so `cat /etc/alloy/config.alloy` and `tail -f /var/log/alloy/stderr.log` work without sudo.

## Why download a prebuilt binary instead of `brew install grafana/grafana/alloy`

The Homebrew formula builds from source. On macOS 14.5 with Command Line Tools 15.3, `go build` fails with:

```
ld: B/BL out of range -148443508 (max +/-128MB) from ...
clang: error: linker command failed with exit code 1
```

Apple's old `ld` in CLT 15.3 can't resolve branch islands for Alloy's ~500 MB Go binary on arm64. Newer CLT (16.x with `ld-prime`) fixes it, but upgrading CLT needs a ~5 GB install and sudo. Tracking issue: [grafana/homebrew-grafana#134](https://github.com/grafana/homebrew-grafana/issues/134). `spawn-claude` sidesteps the problem by fetching the prebuilt `alloy-darwin-arm64.zip` directly from the upstream GitHub release.

## Uninstall

```bash
spawn-claude collector uninstall          # bootout + remove plist and /usr/local/bin/alloy
spawn-claude collector uninstall --purge  # also remove /etc/alloy, /var/lib/alloy, /var/log/alloy
```

## Roadmap

- ~~**PR2** — `spawn-claude run`.~~ ✅ shipped
- ~~**PR3** — Presets, `collector configure`, `run --direct`.~~ ✅ shipped
- ~~**PR4** — `spawn-claude doctor`.~~ ✅ shipped
- ~~**PR5** — goreleaser pipeline and `install.sh` bootstrap.~~ ✅ shipped

## Releasing

Pushing a tag like `v0.1.0` to `main` triggers `.github/workflows/release.yml`, which runs goreleaser and produces a GitHub release with the darwin/arm64 tarball and a `checksums.txt`. The `install.sh` bootstrap resolves the latest release tag and installs from that artifact, so a new release is live as soon as the workflow finishes.

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Development

```bash
go build -o spawn-claude .
go vet ./...
./spawn-claude --help
```

## License

MIT — see [LICENSE](LICENSE).
