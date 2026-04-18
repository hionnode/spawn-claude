# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

A Go CLI (module `github.com/hionnode/spawn-claude`) that manages a system-wide Grafana Alloy LaunchDaemon on macOS and wraps `claude` with OTLP env vars so Claude Code telemetry flows through the local collector to a configurable backend. **macOS / Apple Silicon only.**

The collector lives at the standard Homebrew/`.deb`/`.rpm` paths (`/etc/alloy/config.alloy`, `/usr/local/bin/alloy`, `/var/lib/alloy/data`, `/Library/LaunchDaemons/com.grafana.alloy.plist`) and runs as root. Every privileged file write and `launchctl` call in the `system/` domain self-escalates via `sudo` — the user is prompted inline on install/configure/uninstall.

Pre-1.0. PR1 (this release) ships only the collector lifecycle commands. See `README.md` "Roadmap" for what each subsequent PR adds.

## Build / run / verify

```bash
go build -o spawn-claude .              # single binary; no external runtime deps beyond alloy itself
go vet ./...
./spawn-claude --help
./spawn-claude collector install        # prompts for sudo to write /etc/alloy, /usr/local/bin, and /Library/LaunchDaemons
```

No test suite yet. Verification is manual end-to-end: see the "Verification" section of the PR1 plan (or just cycle `install` → `status` → `reload` → `restart` → `uninstall --purge`).

## Architecture (the big picture)

Two layers, with `cmd/` strictly orchestrating `internal/`:

- **`cmd/`** — cobra command tree. One file per subcommand (`collector_install.go`, `collector_status.go`, …). Files here should be ≤100 lines (install is the exception — it orchestrates the full sudo script); they parse flags, call into `internal/alloy`, and render output. No business logic.
- **`internal/alloy/`** — everything that talks to Alloy or launchd. `paths.go` is the single source of truth for every path (all hardcoded to system locations — nothing else recomputes paths). `sudo.go` (`SudoShell`) runs a `/bin/sh` script under `sudo` with stdio attached so password prompts flow inline; every privileged step funnels through here. `service_darwin.go` (build-tagged `//go:build darwin`) wraps `launchctl bootstrap/bootout/kickstart` in the `system/` domain via `SudoShell`; `Status` uses `launchctl print system/<label>` which doesn't need sudo for reads. `download.go` fetches the pinned release zip from GitHub, extracts with `archive/zip` (no shelling out to `unzip`), then sudo-installs the binary to `/usr/local/bin/alloy`. `ready.go` polls `/-/ready`. `version.go` holds the `DefaultVersion` constant that `--alloy-version` overrides.
- **`internal/assets/`** — embedded static LaunchDaemon plist and the `_base.alloy` placeholder config. `fs.go` declares `//go:embed platform/... presets/...`. The plist has no template tokens — every path in it is an absolute system path.
- **`internal/buildinfo/version.go`** — `var Version = "dev"`, stamped at release time via `-ldflags -X github.com/hionnode/spawn-claude/internal/buildinfo.Version=vX.Y.Z`.

Key invariants:

- Every privileged file/launchctl operation funnels through `alloy.SudoShell`. Bundle steps into one script (chained with `&&` or under `set -e`) so the user is prompted once per command.
- `cmd/collector_install.go` preserves an existing real `/etc/alloy/config.alloy` (never overwrite a user's backend config); only backs up and replaces it if `looksLikeBashEraPlaceholder` matches (small, no `otelcol.*` block). Still: `plutil -lint` before `bootstrap`, poll `/-/ready` for 10s, print last 20 lines of `/var/log/alloy/stderr.log` on timeout.
- `cmd/collector_install.go` also removes any legacy per-user LaunchAgent plist at `~/Library/LaunchAgents/com.grafana.alloy.plist` before loading the new LaunchDaemon, so two Alloys don't fight over 127.0.0.1:12345. Runs without sudo (the file is in `$HOME`).
- `alloy.Bootout` never returns an error — a missing daemon on first install is expected, and we `|| true` inside the shell script too.
- Quarantine xattr stripping runs as `xattr -d com.apple.quarantine ... 2>/dev/null || true` inside the download install script (stdlib has no xattr API).

## Alloy version bump

Edit `internal/alloy/version.go` (`DefaultVersion`). If the upstream zip layout changes, `internal/alloy/download.go::extractAlloyBinary` hardcodes the entry name `alloy-darwin-arm64` and will need updating.

## Historical context

The previous version of this repo was four bash scripts (`install.sh`, `uninstall.sh`, a plist, and a placeholder config). The git history preserves it pre-PR1 if you need to reference it. The Go rewrite exists because the planned CLI surface (telemetry wrapping, vendor presets, doctor) is too large for bash.
