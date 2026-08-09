#!/usr/bin/env bash
#
# go-arch-cli installer — single-command install for Linux and macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/SalvucciFacundo/go-arch-cli/main/install.sh | bash
#
# Behavior:
#   1. Detect OS (linux/darwin) and architecture (amd64/arm64).
#   2. Resolve the latest release tag from the GitHub API.
#   3. Download the matching tarball and its checksums file.
#   4. Verify the tarball SHA-256 against the release checksums.
#   5. Extract and install the `go-arch` binary to /usr/local/bin,
#      falling back to ~/.local/bin when no write permission, and
#      print PATH guidance when the fallback is used.
#
# No build tools are required. The binary is installed as-is.
set -euo pipefail

REPO="SalvucciFacundo/go-arch-cli"
BASE_URL="https://github.com/${REPO}/releases/download"

say() { printf '\033[1;34mgo-arch:\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mgo-arch:\033[0m %s\n' "$*" >&2; exit 1; }

# --- Detect OS / arch -------------------------------------------------------

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux|darwin) ;;
  *) die "unsupported OS: $os (go-arch provides linux and macOS binaries)" ;;
esac

case "$arch" in
  x86_64|amd64) goarch="amd64" ;;
  aarch64|arm64) goarch="arm64" ;;
  *) die "unsupported architecture: $arch" ;;
esac

# --- Resolve latest release -------------------------------------------------

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$@"; }
else
  die "curl is required to install go-arch"
fi

say "Detected ${os}/${goarch}"

LATEST_TAG="$(fetch "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep -oE '"tag_name":\s*"[^"]+"' \
  | head -1 \
  | sed -E 's/.*"([^"]+)"$/\1/')"

[ -n "$LATEST_TAG" ] || die "could not resolve the latest release tag"

VERSION="${LATEST_TAG#v}"
TARBALL="go-arch_${VERSION}_${os}_${goarch}.tar.gz"
CHECKSUMS="go-arch_${VERSION}_checksums.txt"
TARBALL_URL="${BASE_URL}/${LATEST_TAG}/${TARBALL}"
CHECKSUMS_URL="${BASE_URL}/${LATEST_TAG}/${CHECKSUMS}"

say "Latest release: ${LATEST_TAG}"

# --- Download into a temp dir -----------------------------------------------

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

say "Downloading ${TARBALL}..."
fetch "$TARBALL_URL" -o "$TMPDIR/$TARBALL"
fetch "$CHECKSUMS_URL" -o "$TMPDIR/$CHECKSUMS"

# --- Verify checksum --------------------------------------------------------

EXPECTED="$(grep "  ${TARBALL}$" "$TMPDIR/$CHECKSUMS" | awk '{print $1}' || true)"
[ -n "$EXPECTED" ] || die "checksum for ${TARBALL} not found in ${CHECKSUMS}"

ACTUAL="$(sha256sum "$TMPDIR/$TARBALL" | awk '{print $1}')"

if [ "$ACTUAL" != "$EXPECTED" ]; then
  die "checksum mismatch for ${TARBALL}: expected ${EXPECTED}, got ${ACTUAL}"
fi
say "Checksum verified"

# --- Install ----------------------------------------------------------------

tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

if [ -w /usr/local/bin ] || sudo -n true 2>/dev/null; then
  if [ -w /usr/local/bin ]; then
    install -m 0755 "$TMPDIR/go-arch" /usr/local/bin/go-arch
    say "Installed to /usr/local/bin/go-arch"
  else
    sudo install -m 0755 "$TMPDIR/go-arch" /usr/local/bin/go-arch
    say "Installed to /usr/local/bin/go-arch (via sudo)"
  fi
else
  mkdir -p "$HOME/.local/bin"
  install -m 0755 "$TMPDIR/go-arch" "$HOME/.local/bin/go-arch"
  say "Installed to $HOME/.local/bin/go-arch"
  say "NOTE: add $HOME/.local/bin to your PATH, e.g.:"
  say "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
fi

say "Done. Run 'go-arch version' to verify."
