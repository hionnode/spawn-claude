# How spawn-claude works (and how to do it manually)

`spawn-claude` is a thin wrapper around two things macOS already knows how to do:

1. **launchd** runs Grafana Alloy as a per-user background agent.
2. **Claude Code** emits OTLP telemetry when the right `OTEL_*` env vars are set.

The CLI automates the plumbing between them. This document explains the moving parts and shows the exact manual equivalent of every subcommand so you can understand, audit, or bypass it.

## Architecture in one picture

```
  claude (child process)
     │   OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
     ▼
  Alloy (LaunchAgent: com.grafana.alloy)
     │   otelcol.receiver.otlp "default" {
     │     http { endpoint = "127.0.0.1:4318" }
     │     grpc { endpoint = "127.0.0.1:4317" }
     │   }
     ▼
  otelcol.exporter.*   →  stderr.log  (default: local-debug)
                       →  SignOz Cloud / Grafana Cloud / etc. (after `configure`)
```

`spawn-claude run` sets the `OTEL_*` vars and `exec`s `claude`. `spawn-claude collector …` is a façade over `launchctl`, a download step, and some file I/O in `~/.config/alloy/`.

## File layout the CLI owns

| What | Path |
|---|---|
| `spawn-claude` binary | `~/.local/bin/spawn-claude` (placed by `install.sh`) |
| Alloy binary | `~/.local/bin/alloy` |
| Alloy config | `~/.config/alloy/config.alloy` |
| Alloy data / WAL | `~/.config/alloy/data/` |
| Alloy logs | `~/Library/Logs/alloy/{stdout,stderr}.log` |
| LaunchAgent plist | `~/Library/LaunchAgents/com.grafana.alloy.plist` |
| Alloy admin UI | http://127.0.0.1:12345 |
| spawn-claude direct-mode config | `~/.config/spawn-claude/config.toml` |
| Vendor secrets | `~/.config/spawn-claude/secrets.env` (chmod 600) |

Every path is `$HOME`-derived and resolved in `internal/alloy/paths.go`. Nothing is written outside the user's home directory.

## Manual equivalent of `collector install`

Pick an Alloy version (the CLI pins `v1.15.1` in `internal/alloy/version.go`). Adjust if needed.

```bash
VERSION=v1.15.1
mkdir -p ~/.local/bin ~/.config/alloy/data ~/Library/Logs/alloy ~/Library/LaunchAgents

# 1. Download + extract the prebuilt arm64 binary.
curl -fL -o /tmp/alloy.zip \
    "https://github.com/grafana/alloy/releases/download/${VERSION}/alloy-darwin-arm64.zip"
unzip -p /tmp/alloy.zip alloy-darwin-arm64 > ~/.local/bin/alloy
chmod +x ~/.local/bin/alloy
xattr -d com.apple.quarantine ~/.local/bin/alloy 2>/dev/null || true

# 2. Write the default collector config (local-debug: log everything to stderr).
cat > ~/.config/alloy/config.alloy <<'EOF'
otelcol.receiver.otlp "default" {
  grpc { endpoint = "127.0.0.1:4317" }
  http { endpoint = "127.0.0.1:4318" }
  output {
    metrics = [otelcol.exporter.debug.default.input]
    logs    = [otelcol.exporter.debug.default.input]
    traces  = [otelcol.exporter.debug.default.input]
  }
}
otelcol.exporter.debug "default" {
  verbosity = "normal"
}
EOF

# 3. Render the LaunchAgent plist with your $HOME baked in.
cat > ~/Library/LaunchAgents/com.grafana.alloy.plist <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.grafana.alloy</string>
  <key>ProgramArguments</key>
  <array>
    <string>${HOME}/.local/bin/alloy</string>
    <string>run</string>
    <string>--storage.path=${HOME}/.config/alloy/data</string>
    <string>--server.http.listen-addr=127.0.0.1:12345</string>
    <string>--stability.level=experimental</string>
    <string>${HOME}/.config/alloy/config.alloy</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key><false/>
    <key>Crashed</key><true/>
  </dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>${HOME}/Library/Logs/alloy/stdout.log</string>
  <key>StandardErrorPath</key><string>${HOME}/Library/Logs/alloy/stderr.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
  <key>ProcessType</key><string>Background</string>
</dict>
</plist>
EOF
plutil -lint ~/Library/LaunchAgents/com.grafana.alloy.plist

# 4. Load it under your gui/<uid> domain.
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.grafana.alloy.plist 2>/dev/null || true
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.grafana.alloy.plist

# 5. Wait for the admin UI.
until curl -fs http://127.0.0.1:12345/-/ready >/dev/null; do sleep 0.5; done
echo "alloy ready"
```

## Manual equivalent of `collector status / restart / reload / logs`

```bash
# status
launchctl print gui/$(id -u)/com.grafana.alloy | head -20
curl -fs http://127.0.0.1:12345/-/ready

# restart (process restart, config re-read from disk)
launchctl kickstart -k gui/$(id -u)/com.grafana.alloy

# reload (no restart, hot-reload the config)
curl -fX POST http://127.0.0.1:12345/-/reload

# logs
tail -f ~/Library/Logs/alloy/stderr.log
```

## Manual equivalent of `run` (telemetry → local collector)

Just export the OTLP env block and exec claude. These are the exact seven variables `internal/otel/env.go::LocalEnv()` sets:

```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
export OTEL_METRIC_EXPORT_INTERVAL=10000
export OTEL_LOGS_EXPORT_INTERVAL=5000
claude
```

Confirm ingestion by tailing `~/Library/Logs/alloy/stderr.log` while claude runs — you should see OTLP payloads printed by the `debug` exporter.

## Manual equivalent of `collector configure <preset>`

A "preset" is a Go `text/template` rendered into `~/.config/alloy/config.alloy`. Templates live at `internal/assets/presets/*.alloy` and are embedded into the binary at build time. `collector configure` renders, runs `alloy fmt` for validation, atomically swaps the file (backing up the previous one to `config.alloy.bak`), and POSTs `/-/reload`.

### SignOz Cloud

```bash
# 1. Save your ingestion key somewhere the wrapper script below can source it.
ENDPOINT="https://ingest.us.signoz.cloud:443"
KEY="sk_live_..."

# 2. Write the rendered config.
cp ~/.config/alloy/config.alloy ~/.config/alloy/config.alloy.bak
cat > ~/.config/alloy/config.alloy <<EOF
otelcol.receiver.otlp "default" {
  grpc { endpoint = "127.0.0.1:4317" }
  http { endpoint = "127.0.0.1:4318" }
  output {
    metrics = [otelcol.exporter.otlp.signoz.input]
    logs    = [otelcol.exporter.otlp.signoz.input]
    traces  = [otelcol.exporter.otlp.signoz.input]
  }
}
otelcol.exporter.otlp "signoz" {
  client {
    endpoint = "${ENDPOINT}"
    headers  = { "signoz-ingestion-key" = "${KEY}" }
  }
}
EOF

# 3. Validate + hot-reload.
~/.local/bin/alloy fmt ~/.config/alloy/config.alloy >/dev/null
curl -fX POST http://127.0.0.1:12345/-/reload
```

### Grafana Cloud

```bash
OTLP_ENDPOINT="https://otlp-gateway-prod-us-central-0.grafana.net/otlp"
USER="123456"
PASS="glc_..."

cp ~/.config/alloy/config.alloy ~/.config/alloy/config.alloy.bak
cat > ~/.config/alloy/config.alloy <<EOF
otelcol.receiver.otlp "default" {
  grpc { endpoint = "127.0.0.1:4317" }
  http { endpoint = "127.0.0.1:4318" }
  output {
    metrics = [otelcol.exporter.otlphttp.grafana.input]
    logs    = [otelcol.exporter.otlphttp.grafana.input]
    traces  = [otelcol.exporter.otlphttp.grafana.input]
  }
}
otelcol.auth.basic "grafana" {
  username = "${USER}"
  password = "${PASS}"
}
otelcol.exporter.otlphttp "grafana" {
  client {
    endpoint = "${OTLP_ENDPOINT}"
    auth     = otelcol.auth.basic.grafana.handler
  }
}
EOF

~/.local/bin/alloy fmt ~/.config/alloy/config.alloy >/dev/null
curl -fX POST http://127.0.0.1:12345/-/reload
```

## Manual equivalent of `run --direct` (telemetry bypasses the collector)

`--direct` skips the local Alloy entirely and points Claude Code straight at the vendor's OTLP endpoint. The LaunchAgent doesn't even need to be running.

### SignOz Cloud direct

```bash
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_EXPORTER_OTLP_ENDPOINT="https://ingest.us.signoz.cloud:443"
export OTEL_EXPORTER_OTLP_HEADERS="signoz-ingestion-key=sk_live_..."
export OTEL_METRIC_EXPORT_INTERVAL=10000
export OTEL_LOGS_EXPORT_INTERVAL=5000
claude
```

### Grafana Cloud direct

```bash
AUTH=$(printf '%s:%s' "${USER}" "${PASS}" | base64)

export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_METRICS_EXPORTER=otlp
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT="${OTLP_ENDPOINT}"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic ${AUTH}"
export OTEL_METRIC_EXPORT_INTERVAL=10000
export OTEL_LOGS_EXPORT_INTERVAL=5000
claude
```

Exact values come from `internal/presets/definitions.go::signozCloud.DirectEnv` / `grafanaCloud.DirectEnv`.

## Manual equivalent of `doctor`

```bash
command -v claude                                              # claude on PATH
test -x ~/.local/bin/alloy && ~/.local/bin/alloy --version     # alloy installed
launchctl print gui/$(id -u)/com.grafana.alloy >/dev/null      # agent loaded
curl -fs http://127.0.0.1:12345/-/ready                        # admin UI ready
lsof -iTCP:4318 -sTCP:LISTEN                                   # OTLP http port
lsof -iTCP:4317 -sTCP:LISTEN                                   # OTLP grpc port
```

Each check exits non-zero on failure, matching `spawn-claude doctor`'s behavior.

## Manual equivalent of `collector uninstall --purge`

```bash
launchctl bootout gui/$(id -u)/com.grafana.alloy 2>/dev/null || true
rm -f ~/Library/LaunchAgents/com.grafana.alloy.plist
# --purge also removes:
rm -f  ~/.local/bin/alloy
rm -rf ~/.config/alloy
rm -rf ~/Library/Logs/alloy
```

`spawn-claude`'s own binary at `~/.local/bin/spawn-claude` is not touched by `uninstall`; remove it by hand if you want a truly clean slate.

## How releases are cut

1. Push a tag matching `v*` to `main` (`git tag v0.1.1 && git push origin v0.1.1`).
2. `.github/workflows/release.yml` runs goreleaser on a `macos-14` runner.
3. goreleaser builds `darwin/arm64`, tar.gzs the binary, generates `checksums.txt`, and publishes a GitHub release.
4. `install.sh` reads `/repos/hionnode/spawn-claude/releases/latest`, downloads the tarball, verifies its sha256 against `checksums.txt`, and installs to `$BIN_DIR` (default `~/.local/bin`).

No Homebrew tap, no notarization, no installer packages. Build config lives in `.goreleaser.yaml`; to change the binary target or add platforms, edit the `builds:` block there.

## Why Alloy (and not the OTel Collector directly)

Alloy ships as a single ~120 MB prebuilt binary with the full OTel Collector inside plus a cleaner HCL-ish config syntax (`otelcol.receiver.otlp "default" { … }`). The `spawn-claude configure` presets are just Alloy configs with secret placeholders — nothing stops you from hand-writing an arbitrary Alloy config at `~/.config/alloy/config.alloy` and hot-reloading it.

`brew install grafana/grafana/alloy` builds from source, which fails on macOS 14.5 + Command Line Tools 15.3 because Apple's old `ld` can't resolve branch islands for a ~500 MB Go binary. See [grafana/homebrew-grafana#134](https://github.com/grafana/homebrew-grafana/issues/134). `spawn-claude` sidesteps the problem by always pulling the prebuilt `alloy-darwin-arm64.zip` from the upstream GitHub release.
