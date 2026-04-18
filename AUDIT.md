# spawn-claude audit — eng + devex review

**Date:** 2026-04-18
**Scope:** shipped code as of `a2d1b16` (main branch, v0.1.0 released)
**Reviewers:** `/gstack-plan-eng-review` + `/gstack-plan-devex-review`, treating the shipped CLI as the subject under review (not a plan).
**Codebase size:** 1,701 lines of Go across 27 files. Zero tests.

This document consolidates findings from both reviews. Severity: **P1** = security/correctness; **P2** = reliability/UX; **P3** = quality; **P4** = cosmetic. Confidence scores (0-10) reflect how certain the finding is a real issue.

---

## Part 1 — Engineering review

### Architecture

```
[install.sh]                 [GitHub release]
     │ curl | sh                    │
     ▼                              ▼
 ~/.local/bin/spawn-claude ──── goreleaser tarball (sha256-verified ✓)
     │
     ├── collector install ──────► github.com/grafana/alloy/releases (NO CHECKSUM ✗)
     │                              │
     │                              ▼
     │                          ~/.local/bin/alloy
     │
     ├── renders ~/Library/LaunchAgents/com.grafana.alloy.plist
     │        └── launchctl bootstrap gui/<uid>
     │
     ├── run / run --direct ──────► exec claude with OTEL_* env
     │
     ├── collector configure ─────► template → alloy fmt → rename → POST /-/reload
     │
     └── doctor ──────────────────► PATH, binary, agent, /-/ready, :4317, :4318, /metrics
```

Clean layering: `cmd/` orchestrates `internal/`; paths resolved in one place (`internal/alloy/paths.go`); platform-specific code is build-tagged; embedded assets live in `internal/assets/`. "Boring by default" choices everywhere — stdlib `archive/zip`, `embed.FS`, cobra, `net/http`, launchctl shell-outs. Blast radius is tight: all writes are `$HOME`-scoped, no sudo.

#### Findings

| # | Sev | Conf | Location | Finding |
|---|---|---|---|---|
| A1 | **P1** | 9/10 | `internal/alloy/download.go:32` | **No integrity verification on the Alloy binary download.** `install.sh` verifies the spawn-claude tarball against a published checksums.txt, but `downloadFile()` fetches `alloy-darwin-arm64.zip` and writes it to disk without any checksum. If the release URL is hijacked or DNS is poisoned, a malicious binary gets `launchctl bootstrap`-ed as a LaunchAgent. Grafana publishes `sha256sums.txt` alongside every release — fetch it and verify before writing. Largest security gap in the project. |
| A2 | **P2** | 9/10 | `download.go:60`, `collector_reload.go:35`, `run.go:116` | **`http.DefaultClient` with no timeout on multi-minute calls.** The 120 MB Alloy zip download, `/-/reload` POST, and `/-/ready` preflight share the unbounded DefaultClient. `ready.go` sets a 2s client timeout — extend that pattern. Without it, a stalled TCP connection hangs `collector install` indefinitely. |
| A3 | **P2** | 10/10 | `internal/config/config.go:22` | **`Config.Mode` is parsed, validated, and never read.** `cmd/run.go:88-93` uses `cfg.Direct.Vendor` regardless of `cfg.Mode`. A user who sets `mode = "direct"` in their config.toml still gets local-collector behavior from `spawn-claude run` unless they also pass `--direct`. Either wire it or delete it — current state is dead code that lies. |
| A4 | P3 | 7/10 | `cmd/collector_install.go:66` | Race between `Bootout` and `Bootstrap` on reinstall. launchd can be slow; back-to-back calls occasionally fail with "service already loaded." Not observed in practice; one retry with 500ms backoff would harden this. |
| A5 | P4 | 6/10 | `internal/assets/platform/com.grafana.alloy.plist:40` | Plist PATH env includes `/opt/homebrew/bin:/usr/local/bin` but Alloy is launched by absolute path and doesn't shell out. Vestigial from bash-era plist. Harmless; still noise. |

### Code quality

| # | Sev | Conf | Location | Finding |
|---|---|---|---|---|
| Q1 | **P2** | 10/10 | `config.go:22` | Same as A3 — dead `Mode` field. |
| Q2 | P2 | 8/10 | `cmd/collector_configure.go` (162 lines) | Over CLAUDE.md's "`cmd/` files ≤100 lines" guideline. Move `runConfigure`'s body (render → validate → swap → reload) into `internal/presets/apply.go`; cmd shrinks to a flag parser. |
| Q3 | P2 | 7/10 | `cmd/collector_configure.go:133-136` | **`runConfigure` returns nil on `/-/reload` failure, only warning to stderr.** Scripted use thinks configure succeeded when the collector is still serving the old config. Flip to return the error. |
| Q4 | **P2** | 9/10 | `cmd/collector_configure.go:95` + `:113` | **Rendered config.alloy contains the vendor secret in plaintext at mode 0644.** `secrets.env` is 0600-enforced via `EnsureSecureMode`, but `os.WriteFile(alloyPaths.ConfigFile, ..., 0o644)` loses that. Either write 0600 or use Alloy's `sys.env()` in templates so secrets stay in `secrets.env` and Alloy reads them via the plist `EnvironmentVariables` block. |
| Q5 | P3 | 8/10 | `cmd/collector_status.go:45`, `cmd/doctor.go:28` | `os.Exit(1)` inside a cobra `RunE`. Bypasses any future deferred cleanup. Return a sentinel error from `RunE` and let `main.go` handle exit codes. |
| Q6 | P3 | 8/10 | `internal/secrets/env.go:38` | `strings.Trim(v, "\"'")` strips any mix of leading/trailing quote chars. `'"abc"'` silently becomes `abc`. Standard `.env` parsers only strip matched pairs. |
| Q7 | P3 | 8/10 | `internal/presets/definitions.go` | `signozCloud.DirectEnv` and `grafanaCloud.DirectEnv` duplicate 6 of 7 lines. Extract `directEnvBase(protocol, endpoint, headers)` — turns adding a third vendor from 30 lines into 5. |
| Q8 | P3 | 7/10 | repo-wide | Error-wrapping style is inconsistent: some call sites use `%w`, others `%v` after `%s`. Pick one convention. |
| Q9 | **P2** | 7/10 | `cmd/collector_uninstall.go:29` | Uninstall without `--purge` preserves a config.alloy that may contain a vendor ingestion key (see Q4). At minimum warn when preserved config differs from `_base.alloy`; ideally re-render base. |

### Tests

**Coverage: 0/33 branches. Regression risk: high.** `go test ./...` exits successfully because there are no tests — a green CI gives false confidence.

```
CODE PATHS                                                  TESTED
─────────────────────────────────────────────────────────  ──────
cmd/run.go::buildRunEnv (6 branches)                         0/6
internal/presets (Render, Get, DirectEnv contents)           0/4
internal/secrets/env.go (Load, Require, EnsureSecureMode)    0/8
internal/config/config.go (Load, Validate, Save roundtrip)   0/4
internal/otel/env.go (MergeEnv, keyOf)                       0/4
internal/alloy/ready.go (WaitReady, CheckReady)              0/4
internal/alloy/download.go::extractAlloyBinary               0/2
internal/presets/auth.go::basicAuth                          0/1
─────────────────────────────────────────────────────────  ──────
TOTAL                                                       0/33  (0%)
```

**Three critical silent-failure paths with no test:**

1. `otel.MergeEnv` override semantics — one wrong index and Claude telemetry silently breaks.
2. `extractAlloyBinary` zip entry name — if upstream ever renames the binary inside the zip, install fails with a cryptic error and no test catches it.
3. Preset template placeholder drift — if a template renames `SIGNOZ_INGESTION_KEY`, the mismatch ships silently.

**Minimum viable test set (~25 tests, ~20 min of CC work):**
- 4× `otel.MergeEnv` table test (override, append, empty, mixed)
- 5× `secrets.Load` (missing file, comments, quoted, `=` in value, malformed)
- 3× `presets.Render` (happy, missing secret, unknown preset)
- 4× `config.Load/Validate` (missing, direct-no-vendor, bogus mode, roundtrip)
- 4× `ready.go` against `httptest.Server` (200, 500, eventual, timeout)
- 2× `extractAlloyBinary` with in-memory zip (happy, missing entry)
- 3× `buildRunEnv` with a stub preset registry (local, direct happy, direct missing vendor)

None need a real launchd. All are pure unit tests.

**Golden-file test for `run --print-env`** locks in the exact OTLP env var block so no refactor can silently change what Claude Code sees.

### Performance

No hot paths. Install/configure/doctor run in under 3 seconds end-to-end. Alloy download dominates (~20-60s of network). One minor finding:

| # | Sev | Conf | Location | Finding |
|---|---|---|---|---|
| P1 | P3 | 6/10 | `cmd/collector_install.go:110` | `printStderrTail` scans the whole file with a 1MB-buffered scanner to print the last 20 lines. Fine when stderr.log is < 10 KB (install-failure case). If it ever grows past ~100 MB it's wasteful. Read from EOF backward when file size > 10 MB. Low priority. |

### Failure modes

| Scenario | Current behavior | Should |
|---|---|---|
| Alloy release 404 (bad `--alloy-version`) | `download alloy vX: GET ...: status 404` | add hint: "check --alloy-version" |
| DNS blocked / captive portal | `http: dial tcp: lookup ...: no such host` after indeterminate wait | bounded timeout (fix A2) + network hint |
| Partial zip | `alloy-darwin-arm64 not found in zip` (misleading) | checksum verify (fix A1) makes this obvious |
| `/-/reload` 400 after bad config | warning on stderr, exit 0 | return error (fix Q3) |
| secrets.env mode 0644 | warning, continues | default refuse; `--insecure-secrets` escape hatch |
| Two parallel `collector install` runs | undefined (last writer wins) | flock on `~/.config/spawn-claude/install.lock` |

---

## Part 2 — Developer Experience review

### Persona

```
TARGET DEVELOPER PERSONA
========================
Who:       Indie dev / YC-stage founder / platform eng using Claude Code on a Mac
Context:   Saw spawn-claude in a thread or doc; wants observability on their own
           Claude Code usage (cost, tool-call mix, error rates) without standing up
           a full OpenTelemetry Collector by hand.
Tolerance: ~5 min from "curl | sh" to "I see telemetry flowing." Bails if install
           hangs > 30s with no output, or if first real usage requires reading docs.
Expects:   macOS native, one-command install, sensible defaults, clear errors.
           Will likely forward to SignOz or Grafana Cloud because that's what the
           README advertises.
```

### Empathy narrative (actual trace against shipped README)

> I find spawn-claude via the SignOz Claude Code monitoring guide. GitHub README header says "Pre-1.0 but feature-complete." Good signal. I scroll to "Install." One command:
>
> `curl -fsSL https://raw.githubusercontent.com/hionnode/spawn-claude/main/install.sh | sh`
>
> I run it. 5 seconds. "installed ~/.local/bin/spawn-claude" — then a PATH warning I act on. Fine.
>
> Next: `spawn-claude collector install`. It prints `waiting for http://127.0.0.1:12345/-/ready`. Then 20 seconds of silence while Alloy (~120 MB) downloads. No progress bar, no "downloading alloy v1.15.1, this takes about a minute." I start wondering if it's hung. It isn't. "READY." I move on.
>
> `spawn-claude run -- --help`. Claude launches. Is telemetry actually flowing? I can't tell. I run `spawn-claude doctor` in another terminal. All green. OK, now I trust it.
>
> Now I want to send to SignOz. I scroll to "Forwarding telemetry to a real backend." Three commands. I drop my endpoint + key into `~/.config/spawn-claude/secrets.env`, `chmod 600`, run `configure signoz-cloud`. It reloads. I open Claude, use it briefly, check SignOz. Data shows up. Total: ~8 minutes.
>
> I never ran `run --direct` — didn't need to. I briefly wondered what the difference is.

### Competitive benchmark

| Tool | TTHW | Notable DX choice |
|---|---|---|
| Stripe (first charge) | 30s | Test keys included in dashboard, curl example on login |
| Vercel (first deploy) | 2 min | `npx vercel` from any dir just works |
| Honeycomb OTel CLI | 3-5 min | `honeycomb-otelcol install` + one env var |
| Datadog agent | 3 min | One curl, API key, done |
| Raw OTel Collector | 20+ min | Write a YAML config, understand pipelines |
| **spawn-claude (local-debug)** | **~3 min** | `curl\|sh → collector install → run` |
| **spawn-claude (SignOz)** | **~8 min** | `+ secrets.env → configure → reload` |

**Current tier:** Competitive (3-5 min band for the common path). **Reachable tier:** Champion (< 2 min) is possible with a single `spawn-claude quickstart signoz --endpoint X --key Y` command that installs, configures, and runs in one shot.

### Pass-by-pass scorecard

```
+====================================================================+
|                    DX SCORECARD                                     |
+====================================================================+
| Dimension            | Score  | One-line                           |
|----------------------|--------|-------------------------------------|
| Getting Started      | 6/10   | Clear but multi-step, silent dl    |
| API / CLI design     | 8/10   | Strong verbs, consistent, `--`      |
| Error messages       | 8/10   | Every error has a hint (rare!)      |
| Documentation        | 7/10   | README dense; MANUAL.md fills gaps  |
| Upgrade path         | 3/10   | No `upgrade` command, no CHANGELOG  |
| Dev environment      | 5/10   | No completion hint, no --json       |
| Community            | 4/10   | No CONTRIBUTING, no issue templates |
| DX measurement       | 2/10   | No self-telemetry, no TTHW tracking |
+--------------------------------------------------------------------+
| TTHW to local-debug  | ~3 min                                      |
| TTHW to SignOz       | ~8 min                                      |
| Competitive rank     | Competitive (not Champion)                  |
| Overall DX           | 6/10                                        |
+====================================================================+
```

### Key DX findings (concrete, with location)

**D1 — Pass 1, Getting Started, P2 / confidence 9** — `collector install` has no progress output during the 20-60s Alloy download. `internal/alloy/download.go:73` uses plain `io.Copy` to stream the zip. Developers with slow internet think it's hung. Fix: print "downloading alloy v1.15.1 (~120 MB)..." before the GET, and/or wrap the response body in a progress reader that emits `\r` updates every 1 MB.

**D2 — Pass 1, Getting Started, P2 / confidence 8** — No "first success" moment. `spawn-claude run` silently `exec`s `claude`. Developers can't tell whether telemetry is flowing without opening a second terminal. Fix: before `syscall.Exec`, print one line: `forwarding OTLP → http://127.0.0.1:4318` or `--direct → signoz-cloud`. `cmd/run.go:64-66`.

**D3 — Pass 1, Getting Started, P2 / confidence 9** — The SignOz/Grafana path is buried under an H3 in README. That's the real hello-world for 80% of users; `local-debug` is a development convenience. Promote "Forward to SignOz Cloud" to an H2 directly after Install, before the generic quickstart.

**D4 — Pass 1, Getting Started, P3 / confidence 8** — No `spawn-claude quickstart <vendor>` shortcut. A champion-tier install collapses `collector install` + `secrets.env` + `configure signoz-cloud` into one prompted command. `spawn-claude quickstart signoz --endpoint ... --key ...` would hit Champion-tier TTHW (<2 min).

**D5 — Pass 2, API / CLI, P3 / confidence 7** — `run --direct` without a configured vendor errors: `--direct requires [direct].vendor in /Users/chinmay/.config/spawn-claude/config.toml or --vendor=<name>`. Good error. But there's no `spawn-claude run --list-vendors` or similar; discovery requires reading the README or running `collector configure --list`. Cross-reference both commands in the error hint.

**D6 — Pass 3, Error messages, P2 / confidence 8** — `spawn-claude run` without a prior `collector install` errors with "local collector not ready at http://127.0.0.1:12345/-/ready" and suggests `collector install` (`cmd/run.go:125`). Good. But the first-time experience should never reach this state. Fix: `run` detects no plist at all and prints "No collector installed. Run `spawn-claude collector install` first." rather than waiting 2s for a ready check that can never succeed.

**D7 — Pass 3, Error messages — strong finding (8/10 positive).** Every error I triggered (missing secrets, bad preset name, no vendor config) has a hint pointing at the fix. Compared to most pre-1.0 Go CLIs, this is unusually good. Keep it.

**D8 — Pass 4, Docs, P3 / confidence 7** — README has no table of contents. At 150 lines, readers scroll past the quickstart. Add a `## Contents` block after the title.

**D9 — Pass 4, Docs, P3 / confidence 7** — No ASCII architecture diagram in README. MANUAL.md just added one. Port it (or a simpler version) into README so developers see "what does this actually do" on first read.

**D10 — Pass 5, Upgrade path, P1 / confidence 10** — **No `spawn-claude upgrade` command.** Users install once via `curl | sh` then have no way to update except re-running the curl. No CHANGELOG.md, no SemVer policy, no deprecation warnings. At pre-1.0 this is acceptable; at 1.0 it's blocking. Fix now (cheap): add `CHANGELOG.md` and a `spawn-claude upgrade` that re-runs `install.sh`'s logic in-process or just shells out to the same curl-sh.

**D11 — Pass 6, Dev env, P3 / confidence 9** — Cobra ships shell completion (`spawn-claude completion bash|zsh|fish|powershell`) but the README never mentions it. Add a one-liner: "enable tab completion: `spawn-claude completion zsh > ~/.zsh/completions/_spawn-claude`."

**D12 — Pass 6, Dev env, P3 / confidence 8** — No `--json` output on `status` / `doctor`. Scripting and CI integration need structured output. Adding `--json` is ~15 lines per command.

**D13 — Pass 7, Community, P3 / confidence 9** — No CONTRIBUTING.md, no issue templates, no PR template. Single-author right now, but the moment it hits Hacker News that changes. Add both before the first v0.1 post.

**D14 — Pass 8, Measurement, P3 / confidence 9** — spawn-claude has no self-telemetry. Ironic for a telemetry tool. Emit an anonymous (or opt-in) "install succeeded" ping from `install.sh` so you can measure real-world TTHW. Or just instrument `collector install` duration locally.

### Observed in passing

Running `./spawn-claude doctor` during the review surfaced a real broken-state issue on this host: OTLP ports 4317/4318 not listening while the agent is loaded and `/-/ready` returns 200. Doctor caught it cleanly with an exact remediation command: ``hint: try `spawn-claude collector configure local-debug` ``. This is exactly how doctor should work. Strong DX signal.

---

## Part 3 — Ranked action list

Cross-referencing both reviews. Each item names the specific location and effort (human / CC with gstack).

### Critical (P1 — do before declaring anything stable)

1. **Verify SHA256 of downloaded Alloy binary.** `internal/alloy/download.go`. Fetches `sha256sums.txt` from the same release, compare. ~30 LoC. (human: 2h / CC: 20 min)
2. **Start a test suite — the 25 unit tests listed above.** Zero coverage on 1700 LoC of shipped code is high regression risk. (human: 1 day / CC: 20 min)
3. **Add `spawn-claude upgrade` + CHANGELOG.md.** No upgrade story means no 1.0. (human: 1 day / CC: 30 min)

### High priority (P2 — correctness & UX)

4. **Delete or wire `Config.Mode`.** Either make `run` respect `mode = "direct"` or delete the field. (human: 30 min / CC: 10 min)
5. **HTTP timeouts on download + reload.** Copy the `ready.go` pattern. (human: 20 min / CC: 5 min)
6. **`configure` returns error on reload failure.** One-line fix in `collector_configure.go:133`. (human: 5 min / CC: 2 min)
7. **Secrets in config.alloy → mode 0600 or use `sys.env()`.** Current 0644 leaks the key on disk. (human: 1h / CC: 15 min)
8. **Warn on `uninstall` (no-purge) when config.alloy has a vendor preset.** Prevents stale key leftover. (human: 30 min / CC: 10 min)
9. **Progress indication during Alloy download.** Wrap response body in progress reader. (human: 1h / CC: 15 min)
10. **"First success" output from `run`.** One `fmt.Fprintf` before `syscall.Exec`. (human: 5 min / CC: 2 min)

### Medium priority (P3 — quality & polish)

11. **`spawn-claude quickstart <vendor>` one-shot command.** Champion-tier TTHW. (human: 1 day / CC: 45 min)
12. **Promote SignOz path to an H2 in README.** (human: 5 min / CC: 2 min)
13. **Table of contents + architecture diagram in README.** (human: 30 min / CC: 10 min)
14. **Mention `spawn-claude completion` in README.** (human: 2 min / CC: 1 min)
15. **`--json` output on `status` + `doctor`.** (human: 1h / CC: 15 min)
16. **CONTRIBUTING.md, issue + PR templates.** (human: 1h / CC: 15 min)
17. **Move `runConfigure` body out of `cmd/` into `internal/presets/apply.go`.** File-size policy. (human: 30 min / CC: 10 min)
18. **DRY the two `DirectEnv` funcs** via `directEnvBase` helper. (human: 15 min / CC: 5 min)
19. **Better `run` error when no collector installed** (detect plist absence, don't poll /-/ready). (human: 20 min / CC: 10 min)
20. **`bootout → bootstrap` retry with backoff.** (human: 20 min / CC: 10 min)

### Low priority (P4 — cosmetic)

21. Remove vestigial homebrew PATH from plist.
22. Fix `os.Exit(1)` in cobra `RunE` (status, doctor).
23. Standardize error-wrapping style.
24. Strip only matched-pair quotes in `secrets.Load`.

---

## Overall verdict

**STATUS: DONE_WITH_CONCERNS.**

**What's good:** tight layering, boring-tech choices, every error has a hint (rare for pre-1.0), `doctor` actually diagnoses real problems, MANUAL.md documents internals well. The DX is *Competitive*-tier today, which is better than most Go CLIs ship at.

**What blocks 1.0:** unverified Alloy download (P1 security), zero tests (P1 regression risk), no upgrade path (P1 lifecycle).

**Top 3 to do in the next session if you only have an hour:**
1. SHA256 verify the Alloy download (`internal/alloy/download.go`)
2. Delete or wire `Config.Mode` (`config.go`, `run.go`)
3. Start the test suite with `otel.MergeEnv`, `secrets.Load`, `presets.Render`, `buildRunEnv` — the 4 highest-leverage test files. ~20 min of CC work locks in 80% of the silent-regression risk.

If you do those three, the project moves from "shipped but fragile" to "shipped and defensible," and the remaining P2/P3 items become polish work you can land incrementally.
