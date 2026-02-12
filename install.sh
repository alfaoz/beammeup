#!/usr/bin/env bash
set -euo pipefail

# beammeup installer
#
# Usage:
#   curl -fsSL https://beammeup.pw/install.sh | bash
#
# Optional env vars:
#   BEAMMEUP_BASE_URL    (default: https://beammeup.pw)
#   BEAMMEUP_INSTALL_DIR (default: $HOME/.local/bin)
#   BEAMMEUP_VERSION     (default: 1.3.1)

BASE_URL="${BEAMMEUP_BASE_URL:-https://beammeup.pw}"
INSTALL_DIR="${BEAMMEUP_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${BEAMMEUP_VERSION:-1.3.1}"
TARGET="${INSTALL_DIR}/beammeup"
SOURCE_URL="${BASE_URL%/}/beammeup"

die() {
  printf '[beammedown] ERROR: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '[beammedown] %s\n' "$*"
}

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v chmod >/dev/null 2>&1 || die "chmod is required"

mkdir -p "$INSTALL_DIR"
TMP_FILE="$(mktemp -t beammeup.XXXXXX)"
trap 'rm -f "$TMP_FILE"' EXIT

info "beaming down beammeup v${VERSION}"
info "downloading ${SOURCE_URL}"
if ! curl -fsSL "$SOURCE_URL" -o "$TMP_FILE"; then
  die "download failed. Verify TLS/DNS for ${BASE_URL} and that /beammeup exists."
fi

if ! head -n 1 "$TMP_FILE" | grep -q "^#!/usr/bin/env bash"; then
  die "downloaded file does not look like an executable beammeup script."
fi

mv "$TMP_FILE" "$TARGET"
chmod +x "$TARGET"

info "transport complete"
info "installed to ${TARGET}"

case ":$PATH:" in
  *":${INSTALL_DIR}:"*)
    info "run: beammeup"
    ;;
  *)
    info "add this to your shell profile:"
    printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
    info "then run: beammeup"
    ;;
esac
