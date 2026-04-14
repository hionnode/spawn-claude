# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

A small bash-only bundle that installs the prebuilt Grafana Alloy binary as a per-user macOS LaunchAgent. There is no build step, no language toolchain, and no test suite. End goal is a local OTLP collector for Claude Code telemetry — the receiver block hasn't been added to `config.alloy` yet (see README "Next step" section).

## The four files and how they fit

- `install.sh` — guards darwin/arm64, downloads pinned `ALLOY_VERSION` zip from GitHub releases to `~/.local/bin/alloy`, seeds `~/.config/alloy/config.alloy` (only if absent), renders the plist, and `launchctl bootstrap`s it into `gui/$(id -u)`. Polls `http://127.0.0.1:12345/-/ready` for up to 10s.
- `com.grafana.alloy.plist` — template with `__HOME__` placeholders. `install.sh` substitutes via `sed` at install time; never edit the rendered copy in `~/Library/LaunchAgents/` (it gets overwritten). `KeepAlive` restarts on crash but not on clean exit, which is what makes `launchctl kickstart -k` work as a clean restart.
- `config.alloy` — placeholder shipped to users. `install.sh` only copies it if no config exists, so user edits in `~/.config/alloy/config.alloy` survive re-runs.
- `uninstall.sh` — removes the LaunchAgent by default; `--purge` also wipes binary, config, and logs.

## Why prebuilt binary instead of Homebrew

`brew install grafana/grafana/alloy` builds from source, and on this machine's CLT 15.3 the linker fails with "B/BL out of range" on the ~500 MB arm64 binary. Don't switch to brew unless CLT is upgraded to 16.x. See README for the upstream tracking issue.

## Bumping Alloy version

Change `ALLOY_VERSION` at the top of `install.sh`, delete `~/.local/bin/alloy` (the script skips download if the binary exists), and re-run `./install.sh`.

## Verifying changes

```bash
./install.sh                                                  # idempotent; safe to re-run
launchctl list | grep com.grafana.alloy                       # col 2 = last exit code
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:12345/-/ready
curl -X POST http://127.0.0.1:12345/-/reload                  # after editing config
launchctl kickstart -k gui/$(id -u)/com.grafana.alloy         # hard restart
tail -f ~/Library/Logs/alloy/stderr.log
```
