# How spawn-claude works (and how to do it manually)

`spawn-claude` is a thin wrapper around two things macOS already knows how to do:

1. **launchd** runs Grafana Alloy as a system LaunchDaemon (as root).
2. **Claude Code** emits OTLP telemetry when the right `OTEL_*` env vars are set.

The CLI automates the plumbing between them. This document explains the moving parts and shows the exact manual equivalent of every subcommand so you can understand, audit, or bypass it.

## Architecture in one picture

```
  claude (child process)
     │   OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
     ▼
  Alloy (LaunchDaemon: com.grafana.alloy, runs as root)
     │   otelcol.receiver.otlp "default" {
     │     http { endpoint = "127.0.0.1:4318" }
     │     grpc { endpoint = "127.0.0.1:4317" }
     │   }
     ▼
  otelcol.exporter.*   →  /var/log/alloy/stderr.log  (default: local-debug)
                       →  SignOz Cloud / Grafana Cloud / etc. (after `configure`)
```

`spawn-claude run` sets the `OTEL_*` vars and `exec`s `claude`. `spawn-claude collector …` is a façade over `launchctl` (in the `system/` domain, which requires sudo), a download step, and some sudo-protected file I/O under `/etc/alloy/` and `/usr/local/bin/`.

## File layout the CLI owns

| What | Path | Owner |
|---|---|---|
| `spawn-claude` binary | `~/.local/bin/spawn-claude` (placed by `install.sh`) | user |
| Alloy binary | `/usr/local/bin/alloy` | root:wheel 0755 |
| Alloy config | `/etc/alloy/config.alloy` | root:wheel 0644 |
| Alloy data / WAL | `/var/lib/alloy/data/` | root |
| Alloy logs | `/var/log/alloy/{stdout,stderr}.log` | root:wheel 0644 |
| LaunchDaemon plist | `/Library/LaunchDaemons/com.grafana.alloy.plist` | root:wheel 0644 |
| Alloy admin UI | http://127.0.0.1:12345 | |
| spawn-claude direct-mode config | `~/.config/spawn-claude/config.toml` | user |
| Vendor secrets | `~/.config/spawn-claude/secrets.env` (chmod 600) | user |

The system paths (`/etc/alloy`, `/usr/local/bin/alloy`, `/var/lib/alloy`, `/var/log/alloy`, `/Library/LaunchDaemons/...`) match Homebrew / `.deb` / `.rpm` conventions, so vendor onboarding snippets that say "paste into `/etc/alloy/config.alloy`" paste unchanged. Hardcoded in `internal/alloy/paths.go`. Reads of config/logs don't need sudo (0644); writes do.

## Manual equivalent of `collector install`

Pick an Alloy version (the CLI pins `v1.15.1` in `internal/alloy/version.go`). Adjust if needed.

```bash
VERSION=v1.15.1

# 1. Download + extract the prebuilt arm64 binary to a tmpdir (no sudo).
TMP=$(mktemp -d)
curl -fL -o "$TMP/alloy.zip" \
    "https://github.com/grafana/alloy/releases/download/${VERSION}/alloy-darwin-arm64.zip"
unzip -p "$TMP/alloy.zip" alloy-darwin-arm64 > "$TMP/alloy"
chmod +x "$TMP/alloy"

# 2. Install the binary into /usr/local/bin (sudo) and strip quarantine.
sudo install -o root -g wheel -m 0755 "$TMP/alloy" /usr/local/bin/alloy
sudo xattr -d com.apple.quarantine /usr/local/bin/alloy 2>/dev/null || true

# 3. Make the system dirs.
sudo mkdir -p /etc/alloy /var/lib/alloy/data /var/log/alloy

# 4. Write the default collector config (local-debug: log everything to stderr).
#    Only if there isn't already a real config at /etc/alloy/config.alloy.
sudo tee /etc/alloy/config.alloy >/dev/null <<'EOF'
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
sudo chown root:wheel /etc/alloy/config.alloy
sudo chmod 0644 /etc/alloy/config.alloy

# 5. Write the LaunchDaemon plist. Absolute paths everywhere — no $HOME.
sudo tee /Library/LaunchDaemons/com.grafana.alloy.plist >/dev/null <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.grafana.alloy</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/alloy</string>
    <string>run</string>
    <string>--storage.path=/var/lib/alloy/data</string>
    <string>--server.http.listen-addr=127.0.0.1:12345</string>
    <string>--stability.level=experimental</string>
    <string>/etc/alloy/config.alloy</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key><false/>
    <key>Crashed</key><true/>
  </dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>WorkingDirectory</key><string>/var/lib/alloy</string>
  <key>StandardOutPath</key><string>/var/log/alloy/stdout.log</string>
  <key>StandardErrorPath</key><string>/var/log/alloy/stderr.log</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
  <key>ProcessType</key><string>Background</string>
</dict>
</plist>
EOF
sudo chown root:wheel /Library/LaunchDaemons/com.grafana.alloy.plist
sudo chmod 0644 /Library/LaunchDaemons/com.grafana.alloy.plist
sudo plutil -lint /Library/LaunchDaemons/com.grafana.alloy.plist

# 6. Load it in the system domain.
sudo launchctl bootout system/com.grafana.alloy 2>/dev/null || true
sudo launchctl bootstrap system /Library/LaunchDaemons/com.grafana.alloy.plist

# 7. Wait for the admin UI.
until curl -fs http://127.0.0.1:12345/-/ready >/dev/null; do sleep 0.5; done
echo "alloy ready"
```

## Manual equivalent of `collector status / restart / reload / logs`

```bash
# status (reads don't need sudo)
launchctl print system/com.grafana.alloy | head -20
curl -fs http://127.0.0.1:12345/-/ready

# restart (process restart, config re-read from disk)
sudo launchctl kickstart -k system/com.grafana.alloy

# reload (no restart, hot-reload the config — pure HTTP, no sudo)
curl -fX POST http://127.0.0.1:12345/-/reload

# logs (default 0644, no sudo needed)
tail -f /var/log/alloy/stderr.log
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

Confirm ingestion by tailing `/var/log/alloy/stderr.log` while claude runs — you should see OTLP payloads printed by the `debug` exporter.

## Manual equivalent of `collector configure <preset>`

A "preset" is a Go `text/template` rendered into `/etc/alloy/config.alloy`. Templates live at `internal/assets/presets/*.alloy` and are embedded into the binary at build time. `collector configure` renders to a tempfile, runs `alloy fmt` for validation, then sudo-installs atomically (backing up the previous one to `config.alloy.bak`) and POSTs `/-/reload`.

### SignOz Cloud

```bash
ENDPOINT="https://ingest.us.signoz.cloud:443"
KEY="sk_live_..."
STAGE=$(mktemp -t spawn-claude-config).alloy

cat > "$STAGE" <<EOF
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

# Validate (no sudo; alloy binary is 0755).
/usr/local/bin/alloy fmt "$STAGE" >/dev/null

# Back up + install + reload.
sudo cp /etc/alloy/config.alloy /etc/alloy/config.alloy.bak
sudo install -o root -g wheel -m 0644 "$STAGE" /etc/alloy/config.alloy
curl -fX POST http://127.0.0.1:12345/-/reload
```

### Grafana Cloud

```bash
OTLP_ENDPOINT="https://otlp-gateway-prod-us-central-0.grafana.net/otlp"
USER="123456"
PASS="glc_..."
STAGE=$(mktemp -t spawn-claude-config).alloy

cat > "$STAGE" <<EOF
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

/usr/local/bin/alloy fmt "$STAGE" >/dev/null
sudo cp /etc/alloy/config.alloy /etc/alloy/config.alloy.bak
sudo install -o root -g wheel -m 0644 "$STAGE" /etc/alloy/config.alloy
curl -fX POST http://127.0.0.1:12345/-/reload
```

## Manual equivalent of `run --direct` (telemetry bypasses the collector)

`--direct` skips the local Alloy entirely and points Claude Code straight at the vendor's OTLP endpoint. The LaunchDaemon doesn't even need to be running.

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
command -v claude                                                # claude on PATH
test -x /usr/local/bin/alloy && /usr/local/bin/alloy --version   # alloy installed
launchctl print system/com.grafana.alloy >/dev/null              # daemon loaded
curl -fs http://127.0.0.1:12345/-/ready                          # admin UI ready
lsof -iTCP:4318 -sTCP:LISTEN                                     # OTLP http port
lsof -iTCP:4317 -sTCP:LISTEN                                     # OTLP grpc port
# Plus: every running `claude` has CLAUDE_CODE_ENABLE_TELEMETRY=1.
for pid in $(pgrep -u $(id -u) -x claude); do
  ps -E -ww -p $pid -o command= | grep -q CLAUDE_CODE_ENABLE_TELEMETRY=1 \
    || echo "pid $pid is missing OTLP env"
done
```

Each check exits non-zero on failure, matching `spawn-claude doctor`'s behavior.

## Manual equivalent of `collector uninstall --purge`

```bash
sudo launchctl bootout system/com.grafana.alloy 2>/dev/null || true
sudo rm -f /Library/LaunchDaemons/com.grafana.alloy.plist
sudo rm -f /usr/local/bin/alloy
# --purge also removes:
sudo rm -rf /etc/alloy
sudo rm -rf /var/lib/alloy
sudo rm -rf /var/log/alloy
```

`spawn-claude`'s own binary at `~/.local/bin/spawn-claude` is not touched by `uninstall`; remove it by hand if you want a truly clean slate.

## Migrating from the pre-system-install layout

If you have a legacy `~/Library/LaunchAgents/com.grafana.alloy.plist` from an older spawn-claude install, `collector install` bootouts and removes it automatically before loading the new system daemon so two Alloys don't fight over 127.0.0.1:12345. The old `~/.config/alloy/` and `~/Library/Logs/alloy/` directories are left alone; remove them by hand if you want.

## How releases are cut

1. Push a tag matching `v*` to `main` (`git tag v0.1.1 && git push origin v0.1.1`).
2. `.github/workflows/release.yml` runs goreleaser on a `macos-14` runner.
3. goreleaser builds `darwin/arm64`, tar.gzs the binary, generates `checksums.txt`, and publishes a GitHub release.
4. `install.sh` reads `/repos/hionnode/spawn-claude/releases/latest`, downloads the tarball, verifies its sha256 against `checksums.txt`, and installs to `$BIN_DIR` (default `~/.local/bin`).

No Homebrew tap, no notarization, no installer packages. Build config lives in `.goreleaser.yaml`; to change the binary target or add platforms, edit the `builds:` block there.

## Why Alloy (and not the OTel Collector directly)

Alloy ships as a single ~120 MB prebuilt binary with the full OTel Collector inside plus a cleaner HCL-ish config syntax (`otelcol.receiver.otlp "default" { … }`). The `spawn-claude configure` presets are just Alloy configs with secret placeholders — nothing stops you from hand-writing an arbitrary Alloy config at `/etc/alloy/config.alloy` and hot-reloading it.

`brew install grafana/grafana/alloy` builds from source, which fails on macOS 14.5 + Command Line Tools 15.3 because Apple's old `ld` can't resolve branch islands for a ~500 MB Go binary. See [grafana/homebrew-grafana#134](https://github.com/grafana/homebrew-grafana/issues/134). `spawn-claude` sidesteps the problem by always pulling the prebuilt `alloy-darwin-arm64.zip` from the upstream GitHub release.
