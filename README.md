# spawn-claude

Reproducible setup for running Grafana Alloy as a user-level LaunchAgent on macOS (Apple Silicon). The intended next step is wiring Claude Code OTLP telemetry into `config.alloy`.

## Why not `brew install grafana/grafana/alloy`

The Homebrew formula builds from source. On this machine (macOS 14.5, Command Line Tools 15.3) `go build` fails with:

```
ld: B/BL out of range -148443508 (max +/-128MB) from ...
clang: error: linker command failed with exit code 1
```

Apple's old `ld` in CLT 15.3 can't resolve branch islands for Alloy's ~500 MB Go binary on arm64. Newer CLT (16.x, which ships `ld-prime`) fixes it, but upgrading CLT needs a ~5 GB install and `sudo`. Tracking issue: [grafana/homebrew-grafana#134](https://github.com/grafana/homebrew-grafana/issues/134) (Homebrew bottles for alloy).

This bundle skips the build and installs the prebuilt binary from GitHub releases.

## What `install.sh` does

1. Guard: darwin/arm64 only
2. Downloads `alloy-darwin-arm64.zip` at pinned `ALLOY_VERSION` (change one variable at the top of `install.sh` to bump) → `~/.local/bin/alloy`
3. Strips `com.apple.quarantine` xattr
4. Creates `~/.config/alloy/data/` and `~/Library/Logs/alloy/`
5. Drops `config.alloy` into `~/.config/alloy/` only if not already present (preserves your edits)
6. Renders `com.grafana.alloy.plist` (substitutes `__HOME__` → `$HOME`) into `~/Library/LaunchAgents/`
7. `launchctl bootstrap` into `gui/$(id -u)` — starts now and on every login
8. Polls `http://127.0.0.1:12345/-/ready` until HTTP 200 or times out

`KeepAlive` is set to restart on crash but not on clean exits, so `launchctl kickstart -k` cycles cleanly.

## File layout after install

| What | Path |
|---|---|
| Binary | `~/.local/bin/alloy` |
| Config | `~/.config/alloy/config.alloy` |
| Data / WAL | `~/.config/alloy/data/` |
| Logs | `~/Library/Logs/alloy/{stdout,stderr}.log` |
| LaunchAgent | `~/Library/LaunchAgents/com.grafana.alloy.plist` |
| UI | http://127.0.0.1:12345 |

## Install

```bash
cd ~/code/agency/spawn-claude
./install.sh
```

## Day-to-day

```bash
# Check it's up
launchctl list | grep com.grafana.alloy       # col 2 = last exit code (0 = healthy)
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:12345/-/ready

# After editing ~/.config/alloy/config.alloy
curl -X POST http://127.0.0.1:12345/-/reload                # hot reload
launchctl kickstart -k gui/$(id -u)/com.grafana.alloy       # hard restart

# Live logs
tail -f ~/Library/Logs/alloy/stderr.log
```

## Uninstall

```bash
./uninstall.sh          # unload + remove LaunchAgent plist only
./uninstall.sh --purge  # also remove binary, ~/.config/alloy/, and logs
```

## Next step: Claude Code telemetry

Claude Code emits OTLP metrics/traces. Add an `otelcol.receiver.otlp` block to `config.alloy` (gRPC on `:4317`, HTTP on `:4318`), point it at an exporter (Grafana Cloud, local Tempo/Mimir, etc.), then `curl -X POST http://127.0.0.1:12345/-/reload`. Configure Claude Code to send to `http://127.0.0.1:4318` (HTTP) or `:4317` (gRPC).
