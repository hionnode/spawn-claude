#!/usr/bin/env bash
set -euo pipefail

LABEL="com.grafana.alloy"
AGENT_PATH="$HOME/Library/LaunchAgents/$LABEL.plist"
BIN_PATH="$HOME/.local/bin/alloy"
CONFIG_DIR="$HOME/.config/alloy"
LOG_DIR="$HOME/Library/Logs/alloy"

purge=0
for arg in "$@"; do
    case "$arg" in
        --purge) purge=1 ;;
        *) echo "unknown arg: $arg" >&2; exit 2 ;;
    esac
done

uid="$(id -u)"
echo "==> unloading LaunchAgent"
launchctl bootout "gui/$uid" "$AGENT_PATH" 2>/dev/null || true
rm -f "$AGENT_PATH"

if [[ "$purge" -eq 1 ]]; then
    echo "==> --purge: removing binary, config, and logs"
    rm -f "$BIN_PATH"
    rm -rf "$CONFIG_DIR" "$LOG_DIR"
    echo "done. everything removed."
else
    echo "LaunchAgent removed. Preserved:"
    echo "  binary: $BIN_PATH"
    echo "  config: $CONFIG_DIR"
    echo "  logs:   $LOG_DIR"
    echo "Pass --purge to remove those too."
fi
