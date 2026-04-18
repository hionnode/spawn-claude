# Alloy config cookbook for spawn-claude

This is the reference for authoring and evolving `~/.config/alloy/config.alloy` by hand — when the built-in presets (`local-debug`, `signoz-cloud`, `grafana-cloud`) don't cover what you need, or when a backend's UI hands you a config snippet and you're not sure how it merges with what spawn-claude already runs.

For the conceptual tour of spawn-claude itself, see [MANUAL.md](MANUAL.md). Upstream Alloy docs live at https://grafana.com/docs/alloy/.

---

## How spawn-claude launches Alloy

The LaunchAgent at `~/Library/LaunchAgents/com.grafana.alloy.plist` runs this exact command (see `internal/assets/platform/com.grafana.alloy.plist`):

```
~/.local/bin/alloy run \
  --storage.path=~/.config/alloy/data \
  --server.http.listen-addr=127.0.0.1:12345 \
  --stability.level=experimental \
  ~/.config/alloy/config.alloy
```

Two things to notice:

- **Config path is `~/.config/alloy/config.alloy`** — the user-scoped path, not the system-wide `/etc/alloy/config.alloy` that Homebrew/`.deb`/`.rpm` installs use. Vendor UIs that say "paste this into `/etc/alloy/config.alloy`" are wrong for you; paste into the user path or use `spawn-claude collector configure`.
- **`--stability.level=experimental`** is passed globally, so any experimental component (e.g. `otelcol.exporter.debug`) loads. Don't remove this flag unless you know every component in your config is GA-stable.

Reloads are hot. `spawn-claude collector reload` sends `POST http://127.0.0.1:12345/-/reload` to the running process. If the new config is invalid, Alloy keeps serving the old config and the POST returns non-2xx with the parse error. The process is not restarted.

Full restart (rare — only when you change the plist itself, the binary, or CLI flags): `spawn-claude collector restart`.

---

## Config syntax in 60 seconds

Alloy config is declarative. Think "graph of components wired together," not "imperative script."

**Component block:**

```
TYPE "NAME" {
  scalar_arg = "value"
  numeric    = 42
  list       = ["a", "b"]

  nested_block {
    key = "value"
  }
}
```

- `TYPE` is dotted (`otelcol.receiver.otlp`, `prometheus.remote_write`, `loki.write`, etc.).
- `NAME` is an arbitrary label you pick, scoped to the type. `otelcol.receiver.otlp "foo"` and `otelcol.exporter.otlphttp "foo"` coexist fine.

**Wiring components together:**

You reference another component's exports via `TYPE.NAME.EXPORT`:

```
output {
  metrics = [otelcol.exporter.otlphttp.grafana.input]
}
```

The `.input` suffix is the OTel-pipeline consumer handle. Prometheus/Loki components use `.receiver` instead (e.g. `prometheus.remote_write.grafana.receiver`). Which suffix to use is dictated by the component's docs.

**What Alloy config does NOT have:**

- No env-var interpolation by default. `${HOME}` is a literal string. Use the `env("HOME")` function, or use spawn-claude presets (rendered via Go `text/template` before writing).
- No imports, no variables, no conditionals.
- No Terraform-style `locals` or `variable` blocks.

That's it. Everything else is which components exist and how to wire them.

---

## The non-negotiable shape for Claude Code

Claude Code sends OTLP over `http/protobuf` to `http://127.0.0.1:4318` (set by `spawn-claude run`). Your config MUST accept it. Minimum:

```
otelcol.receiver.otlp "default" {
  grpc {
    endpoint = "127.0.0.1:4317"
  }
  http {
    endpoint = "127.0.0.1:4318"
  }
  output {
    metrics = [/* something */]
    logs    = [/* something */]
    traces  = [/* something */]  // can be empty: traces = []
  }
}
```

If port 4318 isn't listening, `spawn-claude doctor` fails and no telemetry reaches any backend. The `output` block must wire to at least one downstream component — you can drop a signal with `[]` but omitting the key is a parse error.

---

## Components you'll actually use

| Component | What it does | Notes |
|---|---|---|
| `otelcol.receiver.otlp` | Accepts OTLP/gRPC (`:4317`) and OTLP/HTTP (`:4318`) | The entry point. Always present. |
| `otelcol.exporter.debug` | Prints every payload to Alloy's stderr | Experimental — kept working by the `--stability.level=experimental` flag in the plist. |
| `otelcol.exporter.otlphttp` | Forwards OTLP over HTTP to an OTLP backend (SignOz Cloud, Grafana Cloud OTLP gateway, New Relic, Honeycomb, etc.) | The cleanest upstream path. Pair with `otelcol.auth.basic` or `otelcol.auth.headers`. |
| `otelcol.exporter.otlp` | Same as above but gRPC | Use if your backend prefers gRPC. |
| `otelcol.auth.basic` | Basic-auth handler referenced by `otelcol.exporter.*` via `auth = otelcol.auth.basic.NAME.handler` | |
| `otelcol.exporter.prometheus` | Bridges OTLP metrics into the Prometheus/`prometheus.remote_write` world | Needed when your backend wants metrics via Prometheus push, not OTLP. |
| `otelcol.exporter.loki` | Bridges OTLP logs into `loki.write` | Same story for logs. |
| `prometheus.remote_write` | Ships metrics to a Prometheus push endpoint | The classic Grafana Cloud metrics path. |
| `loki.write` | Ships logs to a Loki push endpoint | The classic Grafana Cloud logs path. |
| `prometheus.exporter.self` | Exposes Alloy's own internal metrics as a scrape target | For the "Alloy self-monitoring" integration. |
| `prometheus.scrape` | Scrapes a target and forwards to a receiver | Pairs with `prometheus.exporter.self`. |

Full reference: `~/.local/bin/alloy run --help` lists everything the binary knows about; https://grafana.com/docs/alloy/latest/reference/components/ has per-component docs.

---

## Recipes

Drop any of these into `~/.config/alloy/config.alloy`, then `spawn-claude collector reload`. All are complete, standalone configs — not fragments.

### 1. Local debug (the default after `collector install`)

Receives OTLP, dumps every payload to `~/Library/Logs/alloy/stderr.log`, ships nothing upstream. Useful for confirming Claude Code is emitting telemetry at all.

```
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
  verbosity = "normal"   // or "detailed" for full payload dumps
}
```

Tail with `spawn-claude collector logs -f` while running `claude` via `spawn-claude run`. If you see no log lines, the problem is upstream of Alloy (Claude env vars, `spawn-claude run`, or Claude itself).

### 2. OTLP → Grafana Cloud (OTLP gateway)

The simplest Grafana Cloud path. Produces metrics + logs + traces in one pipeline. This is what `spawn-claude collector configure grafana-cloud` renders.

```
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
  username = "YOUR_GRAFANA_INSTANCE_ID"        // numeric string from "Send data → OTLP"
  password = "YOUR_ACCESS_POLICY_TOKEN"         // glc_... token
}

otelcol.exporter.otlphttp "grafana" {
  client {
    endpoint = "https://otlp-gateway-prod-us-central-0.grafana.net/otlp"   // your stack's OTLP gateway URL
    auth     = otelcol.auth.basic.grafana.handler
  }
}
```

Get the three values from Grafana Cloud → your stack → **Connections** → **Send data → OpenTelemetry (OTLP)**. The `endpoint` shown there ends in `/otlp`; paste it verbatim.

### 3. OTLP → SignOz Cloud

```
otelcol.receiver.otlp "default" {
  grpc { endpoint = "127.0.0.1:4317" }
  http { endpoint = "127.0.0.1:4318" }
  output {
    metrics = [otelcol.exporter.otlphttp.signoz.input]
    logs    = [otelcol.exporter.otlphttp.signoz.input]
    traces  = [otelcol.exporter.otlphttp.signoz.input]
  }
}

otelcol.exporter.otlphttp "signoz" {
  client {
    endpoint = "https://ingest.us.signoz.cloud:443"   // your region's ingest host
    headers = {
      "signoz-ingestion-key" = "YOUR_INGESTION_KEY",
    }
  }
}
```

SignOz uses a header, not basic auth. Ingestion key from SignOz → **Settings → Ingestion Settings**.

### 4. OTLP → Prometheus remote_write + Loki push (the "Grafana UI pasted me a huge config" case)

This is what happens when Grafana Cloud's older integration wizards hand you `prometheus.remote_write "metrics_service"` + `loki.write "grafana_cloud_loki"` blocks and expect the rest of the config to reference them. To feed Claude Code's OTLP into those blocks, you need two bridge exporters (`otelcol.exporter.prometheus` and `otelcol.exporter.loki`):

```
otelcol.receiver.otlp "default" {
  grpc { endpoint = "127.0.0.1:4317" }
  http { endpoint = "127.0.0.1:4318" }
  output {
    metrics = [otelcol.exporter.prometheus.default.input]
    logs    = [otelcol.exporter.loki.default.input]
    traces  = []   // drop traces — the Prom/Loki path doesn't carry them
  }
}

otelcol.exporter.prometheus "default" {
  forward_to = [prometheus.remote_write.metrics_service.receiver]
}

otelcol.exporter.loki "default" {
  forward_to = [loki.write.grafana_cloud_loki.receiver]
}

prometheus.remote_write "metrics_service" {
  endpoint {
    url = "https://prometheus-prod-XX-prod-us-central-0.grafana.net/api/prom/push"
    basic_auth {
      username = "YOUR_METRICS_INSTANCE_ID"
      password = "YOUR_TOKEN"
    }
  }
}

loki.write "grafana_cloud_loki" {
  endpoint {
    url = "https://logs-prod-XXX.grafana.net/loki/api/v1/push"
    basic_auth {
      username = "YOUR_LOGS_INSTANCE_ID"
      password = "YOUR_TOKEN"
    }
  }
}
```

The two credentials pairs (metrics instance + logs instance) are different even inside the same Grafana Cloud stack. Grab each from **Connections** → the respective **"Send data via …"** page.

If you want traces too, add a third exporter (`otelcol.exporter.otlp` pointed at Tempo) — Tempo doesn't speak the Prometheus/Loki protocols.

### 5. Alloy self-monitoring on top of any of the above

This is the snippet Grafana's "Alloy integration" wizard generates. It only works if `prometheus.remote_write.metrics_service` and `loki.write.grafana_cloud_loki` (or whatever names it references) are already defined above it — which Recipe 4 does. Append to the bottom of Recipe 4:

```
prometheus.exporter.self "integrations_alloy_health" { }

discovery.relabel "integrations_alloy_health" {
  targets = prometheus.exporter.self.integrations_alloy_health.targets
  rule { target_label = "instance" replacement = constants.hostname }
  rule { target_label = "job"      replacement = "integrations/alloy" }
}

prometheus.scrape "integrations_alloy_health" {
  targets    = discovery.relabel.integrations_alloy_health.output
  forward_to = [prometheus.relabel.integrations_alloy_health.receiver]
  job_name   = "integrations/alloy"
}

prometheus.relabel "integrations_alloy_health" {
  forward_to = [prometheus.remote_write.metrics_service.receiver]
  rule {
    source_labels = ["__name__"]
    regex         = "alloy_build_info|alloy_component_controller_running_components|...|up"
    action        = "keep"
  }
}

logging {
  write_to = [loki.process.logs_integrations_integrations_alloy_health.receiver]
}

loki.process "logs_integrations_integrations_alloy_health" {
  forward_to = [loki.relabel.logs_integrations_integrations_alloy_health.receiver]
  stage.regex {
    expression = "(level=(?P<log_level>[\\s]*debug|warn|info|error))"
  }
  stage.labels {
    values = { level = "log_level" }
  }
}

loki.relabel "logs_integrations_integrations_alloy_health" {
  forward_to = [loki.write.grafana_cloud_loki.receiver]
  rule { target_label = "instance" replacement = constants.hostname }
  rule { target_label = "job"      replacement = "integrations/alloy" }
}
```

Paste the full `regex = ".."` allow-list from Grafana's UI output — it's long and gets edited by Grafana when they add new internal metrics.

---

## Secrets: don't hardcode them

Hardcoding `password = "glc_..."` into `~/.config/alloy/config.alloy` works but puts credentials in a world-readable file (`0o644`). Two better paths:

**Option A — `spawn-claude collector configure <preset>` with `secrets.env`.** Drop credentials into `~/.config/spawn-claude/secrets.env` (chmod 600), then run the appropriate `configure` command. The preset templates live at `internal/assets/presets/*.alloy` and reference keys by Go-template syntax. This is the recommended path for SignOz and Grafana Cloud OTLP flows — see the README.

**Option B — `env()` function for hand-edited configs.** Alloy's `env()` function reads process env vars:

```
otelcol.auth.basic "grafana" {
  username = env("GRAFANA_INSTANCE_ID")
  password = env("GRAFANA_TOKEN")
}
```

For these to resolve, the env vars must be visible to the `alloy` process — meaning you add them to the `<key>EnvironmentVariables</key>` dict in `~/Library/LaunchAgents/com.grafana.alloy.plist` and re-bootstrap the agent. Clunky but doesn't require a spawn-claude preset.

Current plist only exports `PATH`; edit it directly if you go this route, then `launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.grafana.alloy.plist && launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.grafana.alloy.plist`.

---

## Authoring workflow

```bash
# 1. edit
$EDITOR ~/.config/alloy/config.alloy

# 2. validate (formats + parses; -w writes back in place)
~/.local/bin/alloy fmt -w ~/.config/alloy/config.alloy

# 3. hot-reload (non-2xx = config rejected, old config still running)
spawn-claude collector reload

# 4. watch for runtime errors
spawn-claude collector logs -f

# 5. end-to-end smoke
spawn-claude doctor
```

If step 3 fails, read the error body — Alloy returns the parse/validation error inline. If step 4 shows connection refused / 401 / 403, your credentials or endpoint URL is wrong; fix and reload. `doctor` passing means OTLP ports are open and `/-/ready` returns 200 — it does NOT verify that upstream delivery is actually working. For that, watch `spawn-claude collector logs -f` while running `claude` and look for successful export batches (or error spam).

---

## Debugging tools

- **UI at http://127.0.0.1:12345** — component graph (`/graph`), per-component state, live component health, current config (`/config`). Open it. Every component shows its input arguments, current output, and any error.
- **`/metrics`** — Alloy's own Prometheus metrics. Useful metrics: `otelcol_receiver_accepted_*`, `otelcol_exporter_sent_*`, `otelcol_exporter_send_failed_*`. If "accepted" is increasing but "sent" isn't, the problem is downstream.
- **`~/Library/Logs/alloy/stderr.log`** — every eval error, every export failure, every reconnect. `tail -F` it during debugging.
- **`~/.local/bin/alloy fmt`** — fastest way to get a parse error without reloading.
- **`~/.local/bin/alloy run --dry-run /path/to/config.alloy`** — parses + validates but doesn't start the server. Same effect as `fmt` for validation purposes, plus it resolves component dependencies.

---

## Common errors

**`component "X.Y.Z" does not exist or is out of scope`**

You're referencing a component that isn't defined anywhere in the config. Usually means you pasted a snippet (like the Grafana self-monitoring integration) that assumes other blocks exist upstream. Either define the missing blocks, or change the reference.

**`X is experimental; set --stability.level=experimental to use it`**

Only happens if the plist's `--stability.level=experimental` flag got stripped. Re-install with `spawn-claude collector install` to regenerate the plist.

**`dial tcp 127.0.0.1:4318: connect: connection refused`** (from Claude Code or a test client)

The config has no `otelcol.receiver.otlp` block bound to `127.0.0.1:4318`. Run `spawn-claude collector configure local-debug` to reset to a known-good shape, then layer in your changes.

**`401 Unauthorized` / `403 Forbidden` in stderr.log**

Credentials wrong. For OTLP basic auth: username should be the numeric instance ID, password should be the `glc_...` token. For SignOz: header key is `signoz-ingestion-key`, not `Authorization`. For Prometheus remote_write: username is the *metrics* instance ID (different from the OTLP one even inside the same stack).

**Reload returns 200 but no data shows up in the backend**

`spawn-claude collector logs -f` is the answer. Alloy will log every export attempt and every failure. If logs are quiet, metrics/traces aren't flowing in — check that `spawn-claude run` is actually setting the OTLP env vars (`spawn-claude run --print-env`) and that Claude Code is emitting (`CLAUDE_CODE_ENABLE_TELEMETRY=1` is required, set by `run`).

**You edit `config.alloy` directly, then `spawn-claude collector configure <preset>` overwrites it**

Expected. `configure` atomically swaps the file (backup at `.bak`). Either keep your changes in a preset template at `internal/assets/presets/*.alloy` and rebuild spawn-claude, or stop using `configure` for this collector.

---

## Following Grafana Cloud's "install via binary" onboarding path

Grafana Cloud's **Collector → Install Alloy → macOS binary** wizard hands you a three-step sequence that looks like:

```bash
curl -O -L "https://github.com/grafana/alloy/releases/latest/download/alloy-darwin-arm64.zip" \
  && unzip "alloy-darwin-arm64.zip" \
  && chmod a+x "alloy-darwin-arm64"

ARCH="arm64" GCLOUD_... GCLOUD_RW_API_KEY="glc_..." /bin/sh -c "$(curl -fsSL \
  https://storage.googleapis.com/cloud-onboarding/alloy/scripts/install-macos-binary.sh)"

GCLOUD_RW_API_KEY="glc_..." ./grafana-agent-darwin-arm64 \
  --config.file=/etc/grafana-agent/config.yml
```

The third step fails with `zsh: no such file or directory: ./grafana-agent-darwin-arm64`. This is not a local setup problem — Grafana's onboarding flow has two bugs stitched together.

**Bug 1 — the third command references the wrong project.** `grafana-agent-darwin-arm64` is the binary for **Grafana Agent**, the project that preceded Alloy and is now deprecated. You installed **Alloy** in step 1 (binary name `alloy-darwin-arm64`). Different project, different binary name, different CLI, different config format:

| | Grafana Agent (deprecated) | Alloy (what you have) |
|---|---|---|
| Binary | `grafana-agent-darwin-arm64` | `alloy-darwin-arm64` |
| CLI | `--config.file=<path>` | `run <path>` |
| Default config | `/etc/grafana-agent/config.yml` (YAML) | `/etc/alloy/config.alloy` (HCL) |

**Bug 2 — `install-macos-binary.sh` does less than its name implies.** Despite being called "install-macos-binary," the script does **not** install a binary and does **not** register a LaunchDaemon. Reading the source (https://storage.googleapis.com/cloud-onboarding/alloy/scripts/install-macos-binary.sh), all it does is download the config template, `sed` in your `GCLOUD_*` env vars, and `sudo mv` the result into `/etc/alloy/config.alloy` with `root:wheel 0644`. The closing log line "Alloy is ready to run" means "config is in place, now you run the binary yourself." Nothing auto-starts.

### The correct third command

```bash
GCLOUD_RW_API_KEY="glc_..." ~/alloy-darwin-arm64 run \
  --storage.path="$HOME/.alloy-data" \
  --server.http.listen-addr=127.0.0.1:12345 \
  /etc/alloy/config.alloy
```

Env var is required because the Grafana-rendered config references `sys.env("GCLOUD_RW_API_KEY")` for both the Prometheus remote_write and Loki write basic-auth passwords. Config is world-readable (`0644`), so no `sudo` for reading. `--storage.path` points the WAL somewhere writable in `$HOME` (default is `data-alloy/` in CWD, which is annoying). `--server.http.listen-addr` pins the UI to localhost:12345 so Grafana's **Test connection** button finds it.

This foregrounds alloy in your terminal. To keep it running in the background:

```bash
GCLOUD_RW_API_KEY="glc_..." nohup ~/alloy-darwin-arm64 run \
  --storage.path="$HOME/.alloy-data" \
  --server.http.listen-addr=127.0.0.1:12345 \
  /etc/alloy/config.alloy \
  > ~/Library/Logs/alloy-grafana/stdout.log \
  2> ~/Library/Logs/alloy-grafana/stderr.log &
disown
```

Survives the current shell closing. Does **not** survive logout or reboot — for that, write a LaunchAgent (see the spawn-claude install flow for a working template at `internal/assets/platform/com.grafana.alloy.plist`; mirror that structure but point `ProgramArguments` at `~/alloy-darwin-arm64` and add `<key>EnvironmentVariables</key>` to inject `GCLOUD_RW_API_KEY`).

### Verify

```bash
curl -s http://127.0.0.1:12345/-/ready   # → "Alloy is ready."
```

Or hit the UI at http://127.0.0.1:12345 in a browser. Grafana's **Test connection** button in the Collector setup page makes the same `/-/ready` probe.

### This config doesn't route Claude Code telemetry

The config `install-macos-binary.sh` downloads is an **Alloy-self-monitoring-only** setup: `prometheus.exporter.self` scraping Alloy's own runtime metrics, forwarded to your Grafana Cloud Prometheus + Loki. There is **no `otelcol.receiver.otlp` block**, so Claude Code's OTLP payloads have nowhere to land. If the goal is to see Claude Code data in Grafana Cloud:

1. Add an `otelcol.receiver.otlp` block plus `otelcol.exporter.prometheus` / `otelcol.exporter.loki` bridges (Recipe 4 above) to `/etc/alloy/config.alloy` by hand, and reload; OR
2. Abandon this install path and run `spawn-claude collector configure grafana-cloud`, which renders a config with OTLP built in.

The two paths can't coexist on one Alloy process — same binary, same `:12345` port. Pick one.

---

## Data flow: how Alloy ships telemetry to Grafana Cloud

Once the OTLP receiver + bridge exporters are in place (either via Recipe 4 hand-edit or via `add-claude-otlp.sh`), here's what actually happens when Claude Code emits a batch of metrics and logs.

```
Claude Code (via `spawn-claude run`)
   │
   │  OTLP/HTTP — POST http://127.0.0.1:4318/v1/metrics
   │              POST http://127.0.0.1:4318/v1/logs
   │              body: OTLP protobuf, gzip-compressed
   │              auth: none (loopback)
   ▼
otelcol.receiver.otlp "claude_code"      (in Alloy, decodes protobuf into
   │                                      OTel's internal data model)
   │
   ├── metrics ──→ otelcol.exporter.prometheus "claude_code"
   │                 │  translates OTel metrics → Prom semantics:
   │                 │    Counter       → <name>_total
   │                 │    Gauge         → <name>
   │                 │    Histogram     → <name>_bucket / _sum / _count
   │                 │    Resource + metric attrs → Prom labels
   │                 │    Dots in names → underscores
   │                 │    (e.g. claude_code.token.usage → claude_code_token_usage_total)
   │                 ▼
   │               prometheus.remote_write "metrics_service"
   │                 │  writes samples to a local WAL at
   │                 │    ~/.alloy-data/prometheus.remote_write.metrics_service/wal/
   │                 │  batches samples into shards, snappy-compresses,
   │                 │  POSTs to:
   │                 │    https://prometheus-prod-43-prod-ap-south-1.grafana.net/api/prom/push
   │                 │  protocol: Prometheus remote_write v1 (protobuf + snappy)
   │                 │  auth:     basic_auth (username=3055746,
   │                 │            password=sys.env("GCLOUD_RW_API_KEY"))
   │                 │  retries:  5xx + timeouts retried with backoff;
   │                 │            4xx (except 429) drop permanently.
   │
   ├── logs ─────→ otelcol.exporter.loki "claude_code"
   │                 │  translates OTel log records → Loki entries:
   │                 │    log body        → line text
   │                 │    severity + event name → labels (+ "level", etc.)
   │                 │    resource.service.name → stream label
   │                 ▼
   │               loki.write "grafana_cloud_loki"
   │                 │  batches in memory (no on-disk WAL),
   │                 │  snappy-compresses, POSTs to:
   │                 │    https://logs-prod-028.grafana.net/loki/api/v1/push
   │                 │  protocol: Loki push API (protobuf default, JSON fallback)
   │                 │  auth:     basic_auth (username=1523548,
   │                 │            password=sys.env("GCLOUD_RW_API_KEY"))
   │                 │  retries:  same 5xx/timeout policy as remote_write,
   │                 │            but memory-buffered — see failure modes below.
   │
   └── traces ───→ []  (dropped; no Tempo exporter in this config)
```

### Each hop in detail

**Hop 1 — Claude Code → OTLP receiver (loopback).** Pure OTLP/HTTP over TCP to `127.0.0.1:4318`. Two POSTs per export cycle: `/v1/metrics` every 10s, `/v1/logs` every 5s (controlled by `OTEL_METRIC_EXPORT_INTERVAL` and `OTEL_LOGS_EXPORT_INTERVAL`, both set by `spawn-claude run`). Body is OTLP protobuf, gzip-compressed by the SDK by default. No auth — the receiver is bound to loopback so nothing else on the network can reach it. Typical payload size observed: 500 bytes to 10 KB per POST.

**Hop 2 — receiver → bridge exporters (in-process).** The receiver decodes the protobuf into OTel's internal `pdata.Metrics` / `pdata.Logs` structures, then fans out to whatever components are listed in its `output` block. No serialization, no network — this is a Go function call inside the Alloy process. Latency: microseconds.

**Hop 3 — bridge exporters translate formats (in-process).**

- `otelcol.exporter.prometheus` converts OTel metrics into Prometheus samples. OTel's richer metric model (delta counters, exponential histograms, resource attributes) is flattened into Prometheus's simpler model. Lossy in places: delta counters become cumulative via internal bookkeeping; exponential histograms degrade to classical bucket histograms; non-string attributes become stringified labels.
- `otelcol.exporter.loki` flattens OTel log records into Loki lines. Log body becomes the line text. A subset of attributes gets promoted to stream labels. High-cardinality attributes (session IDs, request IDs) stay in the line body, not the labels — which is the right default, but means you can't alert on them without parsing.

**Hop 4 — `prometheus.remote_write` → Grafana Cloud Prometheus.** This is the robust hop. Samples land in a local WAL (write-ahead log) on disk before anything goes over the wire. A pool of "shards" reads from the WAL, batches samples into `remote_write` protobuf messages, snappy-compresses them, and POSTs to your Grafana Cloud Prometheus push URL. Default batch: 500 samples per shard, up to 200 shards. On network failure, samples stay in the WAL and retry with exponential backoff indefinitely — no data loss as long as disk holds up. The `prometheus_remote_storage_*` metrics on Alloy's `/metrics` expose queue depth, retries, dropped samples, and highest-sent timestamp.

**Hop 5 — `loki.write` → Grafana Cloud Loki.** Memory-buffered, not WAL-backed. Log entries batch in RAM (`batch_wait = 1s`, `batch_size = 1MiB` default), snappy-compress, POST to the Loki push URL. On network failure, entries retry from memory up to `max_retries` (default 10). If the queue fills before delivery, new entries drop. In practice Claude Code's log volume is low enough (13 records in a partial session) that this is never a bottleneck.

### Auth, in one place

Both `prometheus.remote_write` and `loki.write` use HTTP basic auth against Grafana Cloud. Username is the **instance ID** (different per datasource — your stack has one for metrics, another for logs). Password is an **access policy token** (`glc_...`). In the config, password is read from the process env via `sys.env("GCLOUD_RW_API_KEY")`, so the token lives only in whatever launches Alloy (shell env, LaunchAgent's `EnvironmentVariables`, or a systemd env file), not in the config file itself.

### Failure modes and what to watch

| Scenario | What happens | What to watch |
|---|---|---|
| Grafana Cloud is down | Metrics: buffered in WAL indefinitely. Logs: buffered in RAM, drop once queue is full. | `prometheus_remote_storage_samples_pending`, `prometheus_remote_storage_samples_dropped_total`, `loki_write_dropped_bytes_total` |
| Invalid credentials (401/403) | Metrics + logs retried briefly, then dropped permanently. Alloy logs the 401 to stderr. | stderr log; `prometheus_remote_storage_samples_failed_total` |
| Network slow but not down | Shards back up, WAL grows, samples get delayed. Prom backend tolerates arbitrary lag; it's the right behavior. | `prometheus_remote_storage_highest_timestamp_in_seconds` vs wall clock — if gap grows, you're behind |
| Claude Code emits gauge with high-cardinality attrs | Prom creates one series per unique combo. With session_id or request_id as a label, series count explodes. Grafana Cloud eventually rejects with `series limit exceeded`. | `prometheus_remote_storage_highest_sent_series`; Grafana Cloud billing dashboard. Drop high-cardinality labels via `prometheus.relabel` if you hit this. |
| Claude Code exits mid-batch | OTLP SDK flushes on shutdown with a 1s timeout. You might lose the last 1-5 seconds of data. | Mostly invisible; check session count matches launches. |

### Verification queries

In Alloy's `/metrics` (http://127.0.0.1:12345/metrics):

```promql
# How much Claude data has the receiver accepted?
otelcol_receiver_accepted_metric_points_total{component_id="otelcol.receiver.otlp.claude_code"}
otelcol_receiver_accepted_log_records_total{component_id="otelcol.receiver.otlp.claude_code"}

# How much has been shipped upstream?
prometheus_remote_storage_samples_total{remote_name=~".*"}
loki_write_sent_bytes_total

# Anything failing?
prometheus_remote_storage_samples_failed_total
otelcol_receiver_refused_metric_points_total{component_id="otelcol.receiver.otlp.claude_code"}
```

In Grafana Cloud (Explore → Prometheus):

```promql
# Usage + cost
sum by (model) (rate(claude_code_token_usage_total[5m]))
sum(increase(claude_code_cost_usage_total[24h]))

# Tool accept/reject ratio
sum by (decision) (rate(claude_code_tool_decision_total[1h]))
```

In Grafana Cloud (Explore → Loki):

```logql
{service_name="claude-code"} | json | event_name="api_request"
{service_name="claude-code"} | json | event_name="api_error"
```

If either the Prom or the Loki query returns no series / no log lines, walk back up the hops (receiver counters first, then exporter counters, then `/-/ready`) until you find the one that's zero. That's where the break is.

---

## When to use presets vs hand-edit

**Presets (`spawn-claude collector configure <name>`)** — use when one of the built-in backends (SignOz Cloud OTLP, Grafana Cloud OTLP gateway, local debug) matches your target. Credentials come from `~/.config/spawn-claude/secrets.env`. Config is validated via `alloy fmt` before swap. `.bak` is kept. This is the low-risk path.

**Hand-edit `~/.config/alloy/config.alloy`** — use when you need something the presets don't cover: multiple backends at once, Prometheus remote_write instead of OTLP, Tempo for traces and OTLP for metrics, relabeling, processors, routing connectors, the self-monitoring integration. Accept that you own the file now and `spawn-claude collector configure` will overwrite it.

If you write a hand-edited config you want to reuse across machines, consider adding it as a preset in `internal/assets/presets/` and opening a PR. That way future you gets the validation + secrets + hot-reload story for free.
