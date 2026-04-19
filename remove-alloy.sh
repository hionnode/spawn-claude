#!/usr/bin/env bash
#
# remove-alloy.sh — nuke every trace of Grafana Alloy from this Mac.
#
# Covers:
#   - spawn-claude's system-scoped LaunchDaemon at /Library/LaunchDaemons
#     (plus any legacy user-scoped LaunchAgent from the pre-migration era)
#   - Homebrew's grafana/grafana/alloy formula (and brew services entry)
#   - Hand-installed binaries in /usr/local/bin, /opt/homebrew/bin, /usr/bin
#   - Any other LaunchDaemons at /Library/LaunchDaemons pointing at alloy
#   - Config/WAL/log directories at ~/.config, /etc, /opt/homebrew/etc,
#     /var/lib, /var/log, ~/Library/Logs
#
# This is the "nuclear" cleanup. `spawn-claude collector uninstall --purge`
# only removes what spawn-claude itself installed; this script also catches
# hand-run binaries, Homebrew formulas, Grafana-onboarding leftovers, and
# any Alloy config dirs left behind by prior installs.
#
# Does NOT touch:
#   - spawn-claude itself (binary at ~/.local/bin/spawn-claude)
#   - ~/.config/spawn-claude/ (secrets, preset selection)
#   - Homebrew taps (brew untap grafana/grafana is up to you)
#
# Idempotent. Re-runnable. Prompts once unless --yes is passed.
# Uses sudo only for paths outside $HOME.
#
# Usage:
#   ./remove-alloy.sh          # show plan, confirm, execute
#   ./remove-alloy.sh --yes    # skip the confirmation prompt
#   ./remove-alloy.sh --dry    # show what would be removed, don't touch anything

set -uo pipefail

YES=0
DRY=0
for arg in "$@"; do
    case "$arg" in
        -y|--yes) YES=1 ;;
        -n|--dry|--dry-run) DRY=1 ;;
        -h|--help)
            sed -n '3,28p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) echo "unknown arg: $arg" >&2; exit 2 ;;
    esac
done

say()  { printf "\n==> %s\n" "$*"; }
note() { printf "    %s\n" "$*"; }
gone() { printf "    [removed]  %s\n" "$*"; }
skip() { printf "    [skip]     %s\n" "$*"; }
would() { printf "    [would rm] %s\n" "$*"; }

HOME_DIR="${HOME:?}"
UID_NUM="$(id -u)"

# Collect everything that exists, so the user can see the plan before committing.
declare -a PLISTS=(
    "$HOME_DIR/Library/LaunchAgents/com.grafana.alloy.plist"
    "$HOME_DIR/Library/LaunchAgents/homebrew.mxcl.grafana-alloy.plist"
    "$HOME_DIR/Library/LaunchAgents/homebrew.mxcl.alloy.plist"
    "/Library/LaunchAgents/com.grafana.alloy.plist"
    "/Library/LaunchDaemons/com.grafana.alloy.plist"
    "/Library/LaunchDaemons/homebrew.mxcl.grafana-alloy.plist"
    "/Library/LaunchDaemons/homebrew.mxcl.alloy.plist"
)

declare -a BINARIES=(
    "$HOME_DIR/.local/bin/alloy"
    "/usr/local/bin/alloy"
    "/opt/homebrew/bin/alloy"
    "/usr/bin/alloy"
)

declare -a CONFIG_DIRS=(
    "$HOME_DIR/.config/alloy"
    "/etc/alloy"
    "/opt/homebrew/etc/alloy"
)

declare -a DATA_DIRS=(
    "$HOME_DIR/Library/Logs/alloy"
    "/var/lib/alloy"
    "/var/log/alloy"
    "/opt/homebrew/var/alloy"
    "/opt/homebrew/var/log/alloy"
)

say "Scanning for alloy installations..."

FOUND_ANY=0
ROW() { printf "    %-8s %s\n" "$1" "$2"; FOUND_ANY=1; }

# running process
if pgrep -x alloy >/dev/null 2>&1; then
    ROW "process" "pid(s): $(pgrep -x alloy | tr '\n' ' ')"
fi

# launchctl entries
LC_ENTRIES="$(launchctl list 2>/dev/null | awk '/alloy/ {print "      " $0}')" || true
if [ -n "${LC_ENTRIES}" ]; then
    printf "    launchd entries:\n%s\n" "$LC_ENTRIES"
    FOUND_ANY=1
fi

for P in "${PLISTS[@]}";     do [ -f "$P" ] && ROW "plist"   "$P"; done
for B in "${BINARIES[@]}";   do [ -e "$B" ] && ROW "binary"  "$B"; done
for D in "${CONFIG_DIRS[@]}"; do [ -d "$D" ] && ROW "config"  "$D"; done
for D in "${DATA_DIRS[@]}";   do [ -d "$D" ] && ROW "data"    "$D"; done

# brew formula
BREW_FORMULA=""
if command -v brew >/dev/null 2>&1; then
    BREW_FORMULA="$(brew list --formula 2>/dev/null | grep -E '^(alloy|grafana-alloy)$' | head -1)"
    if [ -n "$BREW_FORMULA" ]; then
        ROW "brew" "$BREW_FORMULA (brew uninstall --force + brew services stop)"
    fi
fi

if [ "$FOUND_ANY" -eq 0 ]; then
    say "Nothing to remove. System is clean."
    exit 0
fi

if [ "$DRY" -eq 1 ]; then
    say "Dry run. No changes made. Re-run without --dry to actually remove."
    exit 0
fi

if [ "$YES" -ne 1 ]; then
    echo
    read -r -p "Proceed with removal? [y/N] " ANS
    case "${ANS:-}" in
        y|Y|yes|YES) ;;
        *) echo "Aborted."; exit 1 ;;
    esac
fi

# Warm up sudo once if we're going to need it for any system path.
NEEDS_SUDO=0
for P in "${PLISTS[@]}" "${BINARIES[@]}" "${CONFIG_DIRS[@]}" "${DATA_DIRS[@]}"; do
    case "$P" in
        "$HOME_DIR"/*) continue ;;
        *) [ -e "$P" ] && NEEDS_SUDO=1 ;;
    esac
done
if [ "$NEEDS_SUDO" -eq 1 ]; then
    say "Some paths require sudo (system-wide installs). You'll be prompted."
    sudo -v || { echo "sudo required, aborting"; exit 1; }
fi

# Helper: rm that uses sudo iff path is outside $HOME.
nuke() {
    local path="$1"
    case "$path" in
        "$HOME_DIR"/*) rm -rf "$path" ;;
        *)             sudo rm -rf "$path" ;;
    esac
}

say "1. Stopping running alloy processes"
if pgrep -x alloy >/dev/null 2>&1; then
    pkill -x alloy 2>/dev/null && note "SIGTERM sent"
    sleep 1
    if pgrep -x alloy >/dev/null 2>&1; then
        pkill -9 -x alloy 2>/dev/null && note "SIGKILL sent"
    fi
else
    skip "no alloy process running"
fi

say "2. Unloading launchd services"
# spawn-claude's user agent (label form — preferred, survives stale plists)
launchctl bootout "gui/$UID_NUM/com.grafana.alloy" 2>/dev/null && note "bootout gui/$UID_NUM/com.grafana.alloy" || true
# brew's user agent (if brew services installed it)
launchctl bootout "gui/$UID_NUM/homebrew.mxcl.grafana-alloy" 2>/dev/null && note "bootout homebrew.mxcl.grafana-alloy (user)" || true
launchctl bootout "gui/$UID_NUM/homebrew.mxcl.alloy" 2>/dev/null && note "bootout homebrew.mxcl.alloy (user)" || true
# system scope
sudo launchctl bootout "system/com.grafana.alloy" 2>/dev/null && note "bootout system/com.grafana.alloy" || true
sudo launchctl bootout "system/homebrew.mxcl.grafana-alloy" 2>/dev/null && note "bootout system/homebrew.mxcl.grafana-alloy" || true
sudo launchctl bootout "system/homebrew.mxcl.alloy" 2>/dev/null && note "bootout system/homebrew.mxcl.alloy" || true

say "3. Removing plist files"
for P in "${PLISTS[@]}"; do
    if [ -f "$P" ]; then
        nuke "$P" && gone "$P"
    fi
done

say "4. Uninstalling Homebrew formula"
if [ -n "$BREW_FORMULA" ]; then
    brew services stop "$BREW_FORMULA" 2>/dev/null || true
    brew uninstall --force "$BREW_FORMULA" 2>/dev/null && gone "brew formula $BREW_FORMULA" || note "brew uninstall $BREW_FORMULA failed (check manually)"
else
    skip "no brew formula installed"
fi

say "5. Removing stray binaries"
for B in "${BINARIES[@]}"; do
    if [ -L "$B" ] && [ ! -e "$B" ]; then
        # dangling symlink (common leftover from brew uninstall)
        nuke "$B" && gone "dangling symlink $B"
    elif [ -e "$B" ]; then
        nuke "$B" && gone "$B"
    fi
done

say "6. Removing config directories"
for D in "${CONFIG_DIRS[@]}"; do
    if [ -d "$D" ]; then
        nuke "$D" && gone "$D"
    fi
done

say "7. Removing data / log directories"
for D in "${DATA_DIRS[@]}"; do
    if [ -d "$D" ]; then
        nuke "$D" && gone "$D"
    fi
done

say "8. Verifying clean state"
LINGERING=0

if command -v alloy >/dev/null 2>&1; then
    note "WARN: 'alloy' still resolves to $(command -v alloy)"
    LINGERING=1
fi
if launchctl list 2>/dev/null | grep -qi alloy; then
    note "WARN: launchctl still lists alloy:"
    launchctl list 2>/dev/null | grep -i alloy | sed 's/^/      /'
    LINGERING=1
fi
for D in "${CONFIG_DIRS[@]}" "${DATA_DIRS[@]}"; do
    if [ -d "$D" ]; then
        note "WARN: directory still present: $D"
        LINGERING=1
    fi
done
for P in "${PLISTS[@]}"; do
    if [ -f "$P" ]; then
        note "WARN: plist still present: $P"
        LINGERING=1
    fi
done

echo
if [ "$LINGERING" -eq 0 ]; then
    say "Done. No alloy binaries, services, configs, or data remaining."
else
    say "Done with warnings. Investigate the lingering items above."
fi

cat <<EOF

Next steps:
  - spawn-claude itself is untouched. If you re-install alloy via Grafana's docs
    at a different path, 'spawn-claude doctor' will still expect the binary at
    ~/.local/bin/alloy and the config at ~/.config/alloy/config.alloy. You'll
    either want to reinstall via spawn-claude ('spawn-claude collector install')
    or point it at the new paths (out of scope today).
  - ~/.config/spawn-claude/ was preserved (contains your secrets.env and
    direct-vendor selection). Delete manually if desired.
  - If you tapped grafana/grafana for Homebrew, the tap itself was NOT removed.
    Run 'brew untap grafana/grafana' if you want to fully detach.
EOF
