#!/usr/bin/env bash
set -euo pipefail

ALLOY_VERSION="v1.15.1"
LABEL="com.grafana.alloy"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN_DIR="$HOME/.local/bin"
BIN_PATH="$BIN_DIR/alloy"
CONFIG_DIR="$HOME/.config/alloy"
CONFIG_FILE="$CONFIG_DIR/config.alloy"
DATA_DIR="$CONFIG_DIR/data"
LOG_DIR="$HOME/Library/Logs/alloy"
AGENT_DIR="$HOME/Library/LaunchAgents"
AGENT_PATH="$AGENT_DIR/$LABEL.plist"
UI_URL="http://127.0.0.1:12345"

os="$(uname -s)"
arch="$(uname -m)"
if [[ "$os" != "Darwin" || "$arch" != "arm64" ]]; then
    echo "error: this installer is darwin/arm64 only (detected $os/$arch)" >&2
    exit 1
fi

mkdir -p "$BIN_DIR" "$DATA_DIR" "$LOG_DIR" "$AGENT_DIR"

if [[ ! -x "$BIN_PATH" ]]; then
    echo "==> downloading alloy $ALLOY_VERSION"
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    url="https://github.com/grafana/alloy/releases/download/$ALLOY_VERSION/alloy-darwin-arm64.zip"
    curl -fL -o "$tmp/alloy.zip" "$url"
    unzip -o -q "$tmp/alloy.zip" -d "$tmp"
    mv "$tmp/alloy-darwin-arm64" "$BIN_PATH"
    chmod +x "$BIN_PATH"
    xattr -d com.apple.quarantine "$BIN_PATH" 2>/dev/null || true
else
    echo "==> alloy binary already present at $BIN_PATH (skipping download)"
fi

if [[ ! -e "$CONFIG_FILE" ]]; then
    echo "==> installing placeholder config to $CONFIG_FILE"
    cp "$SCRIPT_DIR/config.alloy" "$CONFIG_FILE"
else
    echo "==> config exists at $CONFIG_FILE (preserving)"
fi

echo "==> rendering LaunchAgent plist to $AGENT_PATH"
sed "s|__HOME__|$HOME|g" "$SCRIPT_DIR/$LABEL.plist" > "$AGENT_PATH"
plutil -lint "$AGENT_PATH" >/dev/null

uid="$(id -u)"
echo "==> (re)loading LaunchAgent"
launchctl bootout "gui/$uid" "$AGENT_PATH" 2>/dev/null || true
launchctl bootstrap "gui/$uid" "$AGENT_PATH"

echo "==> waiting for $UI_URL/-/ready"
for i in 1 2 3 4 5 6 7 8 9 10; do
    if curl -fs "$UI_URL/-/ready" >/dev/null 2>&1; then
        echo "READY ($UI_URL)"
        exit 0
    fi
    sleep 1
done

echo "error: alloy did not become ready; last 20 lines of stderr.log:" >&2
tail -n 20 "$LOG_DIR/stderr.log" >&2 || true
exit 1
