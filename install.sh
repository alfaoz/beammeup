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
#   BEAMMEUP_VERSION     (default: 1.4.0)

BASE_URL="${BEAMMEUP_BASE_URL:-https://beammeup.pw}"
INSTALL_DIR="${BEAMMEUP_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${BEAMMEUP_VERSION:-1.4.0}"
TARGET="${INSTALL_DIR}/beammeup"
SOURCE_URL="${BASE_URL%/}/beammeup"

die() {
  printf '[beammedown] ERROR: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '[beammedown] %s\n' "$*"
}

run_with_privilege() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
    return $?
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return $?
  fi
  return 1
}

install_dialog() {
  if command -v dialog >/dev/null 2>&1; then
    info "dialog already installed"
    return 0
  fi

  info "installing dialog for full tui mode..."

  if command -v brew >/dev/null 2>&1; then
    brew install dialog || die "failed to install dialog with brew. Run: brew install dialog"
  elif command -v apt-get >/dev/null 2>&1; then
    run_with_privilege apt-get update || die "failed to run apt-get update. Run manually: sudo apt-get update"
    run_with_privilege apt-get install -y dialog || die "failed to install dialog. Run manually: sudo apt-get install -y dialog"
  elif command -v dnf >/dev/null 2>&1; then
    run_with_privilege dnf install -y dialog || die "failed to install dialog. Run manually: sudo dnf install -y dialog"
  elif command -v yum >/dev/null 2>&1; then
    run_with_privilege yum install -y dialog || die "failed to install dialog. Run manually: sudo yum install -y dialog"
  elif command -v pacman >/dev/null 2>&1; then
    run_with_privilege pacman -Sy --noconfirm dialog || die "failed to install dialog. Run manually: sudo pacman -Sy --noconfirm dialog"
  elif command -v zypper >/dev/null 2>&1; then
    run_with_privilege zypper --non-interactive install dialog || die "failed to install dialog. Run manually: sudo zypper --non-interactive install dialog"
  elif command -v apk >/dev/null 2>&1; then
    run_with_privilege apk add dialog || die "failed to install dialog. Run manually: sudo apk add dialog"
  else
    die "dialog not found and no supported package manager detected. Install dialog manually, then run beammeup."
  fi

  command -v dialog >/dev/null 2>&1 || die "dialog install finished but command is still missing."
  info "dialog ready"
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

install_dialog

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
