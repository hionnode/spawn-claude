#!/usr/bin/env bash
#
# add-claude-otlp.sh — extend /etc/alloy/config.alloy so Claude Code OTLP
# telemetry flows through the existing Grafana Cloud Prom + Loki pipeline.
#
# What it adds:
#   - otelcol.receiver.otlp "claude_code" on 127.0.0.1:4317 (gRPC) + :4318 (HTTP)
#   - otelcol.exporter.prometheus "claude_code" → prometheus.remote_write.metrics_service
#   - otelcol.exporter.loki "claude_code"       → loki.write.grafana_cloud_loki
#
# What it doesn't add:
#   - Traces routing. Grafana Cloud Tempo needs a separate OTLP exporter with
#     its own endpoint; add one yourself if you want traces.
#
# Prerequisites:
#   - /etc/alloy/config.alloy already contains the Grafana onboarding default
#     shape (prometheus.remote_write "metrics_service" and loki.write
#     "grafana_cloud_loki" blocks).
#   - Alloy is running on 127.0.0.1:12345.
#
# Idempotent: exits 0 if the block is already present.
# Atomic:   validates via `alloy fmt` and `alloy validate` before swap.
# Reversible: backs up the current config to /etc/alloy/config.alloy.<timestamp>.bak
#
# Usage:
#   ./add-claude-otlp.sh         # add + hot-reload
#   ./add-claude-otlp.sh --dry   # show what would be added, don't change anything

set -euo pipefail

CONFIG=/etc/alloy/config.alloy
ALLOY_BIN="${ALLOY_BIN:-$HOME/alloy-darwin-arm64}"
UI="http://127.0.0.1:12345"
MARKER='otelcol.receiver.otlp "claude_code"'
DRY=0

for arg in "$@"; do
    case "$arg" in
        -n|--dry|--dry-run) DRY=1 ;;
        -h|--help) sed -n '3,28p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) echo "unknown arg: $arg" >&2; exit 2 ;;
    esac
done

say()  { printf "\n==> %s\n" "$*"; }
note() { printf "    %s\n" "$*"; }
fail() { printf "error: %s\n" "$*" >&2; exit 1; }

[ -f "$CONFIG" ] || fail "$CONFIG not found. Run Grafana's install-macos-binary.sh first."

# /etc/alloy/config.alloy is root:wheel 0644 (world-readable), so no sudo for read.
if grep -q -F "$MARKER" "$CONFIG"; then
    say "Claude Code OTLP block already present in $CONFIG. Nothing to do."
    exit 0
fi

grep -q 'prometheus.remote_write "metrics_service"' "$CONFIG" || \
    fail "$CONFIG missing prometheus.remote_write \"metrics_service\" block. Wrong shape, aborting."
grep -q 'loki.write "grafana_cloud_loki"' "$CONFIG" || \
    fail "$CONFIG missing loki.write \"grafana_cloud_loki\" block. Wrong shape, aborting."

TMP="$(mktemp -t alloy-config-XXXXXX).alloy"
trap 'rm -f "$TMP"' EXIT

cp "$CONFIG" "$TMP"
cat >> "$TMP" <<'EOF'

// ---------------------------------------------------------------------------
// Claude Code OTLP telemetry (added by add-claude-otlp.sh).
//
// Receives OTLP from Claude Code on 127.0.0.1:4317 (gRPC) and :4318 (HTTP),
// then bridges metrics into the existing prometheus.remote_write pipeline
// and logs into the existing loki.write pipeline. Traces are dropped.
//
// To feed Claude Code into this receiver:
//   spawn-claude run -- -p 'hello'
// Or manually:
//   CLAUDE_CODE_ENABLE_TELEMETRY=1 \
//   OTEL_METRICS_EXPORTER=otlp OTEL_LOGS_EXPORTER=otlp \
//   OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
//   OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
//   claude
// ---------------------------------------------------------------------------

otelcol.receiver.otlp "claude_code" {
  grpc {
    endpoint = "127.0.0.1:4317"
  }
  http {
    endpoint = "127.0.0.1:4318"
  }
  output {
    metrics = [otelcol.exporter.prometheus.claude_code.input]
    logs    = [otelcol.exporter.loki.claude_code.input]
    traces  = []
  }
}

otelcol.exporter.prometheus "claude_code" {
  forward_to = [prometheus.remote_write.metrics_service.receiver]
}

otelcol.exporter.loki "claude_code" {
  forward_to = [loki.write.grafana_cloud_loki.receiver]
}
EOF

say "Validating proposed config"
if [ -x "$ALLOY_BIN" ]; then
    "$ALLOY_BIN" fmt "$TMP" > /dev/null || fail "alloy fmt rejected the new config"
    note "alloy fmt ok"
    if "$ALLOY_BIN" validate --stability.level=experimental "$TMP" > /dev/null 2>&1; then
        note "alloy validate --stability.level=experimental ok"
    elif "$ALLOY_BIN" validate "$TMP" > /dev/null 2>&1; then
        note "alloy validate ok"
    else
        fail "alloy validate rejected the new config"
    fi
else
    note "warn: $ALLOY_BIN not found; skipping validation (install path may differ)"
fi

if [ "$DRY" -eq 1 ]; then
    say "Dry run. Proposed config is at:"
    note "$TMP"
    note "(diff vs current: diff -u $CONFIG $TMP)"
    trap - EXIT
    exit 0
fi

say "Backing up current config"
BAK="/etc/alloy/config.alloy.$(date +%Y%m%d-%H%M%S).bak"
sudo cp -p "$CONFIG" "$BAK"
note "$BAK"

say "Installing new config"
sudo install -o root -g wheel -m 0644 "$TMP" "$CONFIG"
note "wrote $CONFIG"

say "Hot-reloading Alloy"
if HTTP_CODE=$(curl -sS -o /tmp/alloy-reload.out -w "%{http_code}" -X POST "$UI/-/reload"); then
    if [ "$HTTP_CODE" = "200" ]; then
        note "POST /-/reload → 200"
    else
        echo "warn: /-/reload returned $HTTP_CODE" >&2
        cat /tmp/alloy-reload.out >&2
        echo >&2
        echo "old config is still running; new config rejected" >&2
        exit 1
    fi
else
    echo "warn: curl failed; is alloy running on $UI?" >&2
    exit 1
fi

say "Verifying OTLP ports are listening"
sleep 1
for addr_port in "127.0.0.1 4317" "127.0.0.1 4318"; do
    if nc -z -w1 $addr_port 2>/dev/null; then
        note "[ok]   ${addr_port/ /:} accepting connections"
    else
        note "[warn] ${addr_port/ /:} not yet accepting connections"
    fi
done

cat <<EOF

==> Done. Claude Code OTLP pipeline is live.

Next step — send some data:
  spawn-claude run -- -p 'hello world'

Then check metrics in Grafana Cloud (Explore → Prometheus):
  {service_name=~".*claude.*"}

Restore the previous config if you need to:
  sudo cp $BAK $CONFIG && curl -fX POST $UI/-/reload
EOF
