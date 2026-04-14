# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

A Go CLI (module `github.com/hionnode/spawn-claude`) that manages a per-user Grafana Alloy LaunchAgent on macOS and — in upcoming releases — will wrap `claude` with OTLP env vars so Claude Code telemetry flows through the local collector to a configurable backend. **macOS / Apple Silicon only.**

Pre-1.0. PR1 (this release) ships only the collector lifecycle commands. See `README.md` "Roadmap" for what each subsequent PR adds.

## Build / run / verify

```bash
go build -o spawn-claude .              # single binary; no external runtime deps beyond alloy itself
go vet ./...
./spawn-claude --help
./spawn-claude collector install        # behaviorally identical to the old install.sh
```

No test suite yet. Verification is manual end-to-end: see the "Verification" section of the PR1 plan (or just cycle `install` → `status` → `reload` → `restart` → `uninstall --purge`).

## Architecture (the big picture)

Two layers, with `cmd/` strictly orchestrating `internal/`:

- **`cmd/`** — cobra command tree. One file per subcommand (`collector_install.go`, `collector_status.go`, …). Files here should be ≤100 lines; they parse flags, call into `internal/alloy`, and render output. No business logic.
- **`internal/alloy/`** — everything that talks to Alloy or launchd. `paths.go` is the single source of truth for every `$HOME`-derived path (nothing else recomputes paths). `service_darwin.go` (build-tagged `//go:build darwin`) wraps `launchctl bootstrap/bootout/kickstart/list`. `download.go` fetches the pinned release zip from GitHub and extracts with `archive/zip` — no shelling out to `unzip`. `ready.go` polls `/-/ready`. `version.go` holds the `DefaultVersion` constant that `--alloy-version` overrides.
- **`internal/assets/`** — embedded plist template and the `_base.alloy` placeholder config. `fs.go` declares `//go:embed platform/... presets/...`. Must live here (not at module root) because `//go:embed` can't use `..` and `main.go` already owns the root `package main`.
- **`internal/buildinfo/version.go`** — `var Version = "dev"`, stamped at release time via `-ldflags -X github.com/hionnode/spawn-claude/internal/buildinfo.Version=vX.Y.Z`.

Key invariants:

- `cmd/collector_install.go` must preserve exact behavioral parity with the old bash `install.sh` — specifically: skip download if the binary already exists, preserve an existing `~/.config/alloy/config.alloy`, render `__HOME__` in the plist via `strings.ReplaceAll`, `plutil -lint` before loading, `bootout` (ignore error) then `bootstrap`, poll `/-/ready` for 10s, print last 20 lines of `stderr.log` on timeout.
- `alloy.Bootout` never returns an error — a missing agent on first install is expected.
- Quarantine xattr stripping shells out to `xattr -d` (stdlib has no xattr API) and ignores failure, matching the `|| true` in the old `install.sh`.

## Alloy version bump

Edit `internal/alloy/version.go` (`DefaultVersion`). If the upstream zip layout changes, `internal/alloy/download.go::extractAlloyBinary` hardcodes the entry name `alloy-darwin-arm64` and will need updating.

## Historical context

The previous version of this repo was four bash scripts (`install.sh`, `uninstall.sh`, a plist, and a placeholder config). The git history preserves it pre-PR1 if you need to reference it. The Go rewrite exists because the planned CLI surface (telemetry wrapping, vendor presets, doctor) is too large for bash.
