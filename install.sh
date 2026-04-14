#!/usr/bin/env sh
# Bootstrap installer for spawn-claude.
# Usage:
#     curl -fsSL https://raw.githubusercontent.com/hionnode/spawn-claude/main/install.sh | sh
#
# Fetches the latest release tarball from GitHub and drops the binary at
# $BIN_DIR (default ~/.local/bin). Override with BIN_DIR=/usr/local/bin sh install.sh.
set -eu

REPO="hionnode/spawn-claude"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

os="$(uname -s)"
arch="$(uname -m)"
if [ "$os" != "Darwin" ] || [ "$arch" != "arm64" ]; then
    echo "error: spawn-claude currently supports darwin/arm64 only (detected $os/$arch)" >&2
    exit 1
fi

version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
if [ -z "$version" ]; then
    echo "error: could not resolve latest release from https://api.github.com/repos/$REPO/releases/latest" >&2
    exit 1
fi

tarball="spawn-claude_${version#v}_darwin_arm64.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$tarball"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "==> downloading spawn-claude $version"
curl -fL -o "$tmp/$tarball" "$url"

echo "==> verifying checksum"
curl -fL -o "$tmp/checksums.txt" "https://github.com/$REPO/releases/download/$version/checksums.txt"
(cd "$tmp" && grep " $tarball\$" checksums.txt | shasum -a 256 -c -) \
    || { echo "error: checksum mismatch" >&2; exit 1; }

tar -xzf "$tmp/$tarball" -C "$tmp"

mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/spawn-claude" "$BIN_DIR/spawn-claude"

# Strip quarantine xattr so Gatekeeper doesn't block on first run. The binary
# is not notarized (no Apple Developer account); users who need full signing
# should `go install github.com/hionnode/spawn-claude@latest` instead.
xattr -d com.apple.quarantine "$BIN_DIR/spawn-claude" 2>/dev/null || true

echo "==> installed $BIN_DIR/spawn-claude"

case ":$PATH:" in
    *:"$BIN_DIR":*) ;;
    *)
        echo ""
        echo "note: $BIN_DIR is not on your PATH. Add this to your shell profile:"
        echo "    export PATH=\"$BIN_DIR:\$PATH\""
        ;;
esac

echo ""
echo "Next: spawn-claude collector install"
