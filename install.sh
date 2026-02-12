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
#   BEAMMEUP_VERSION     (default: 1.4.1)

BASE_URL="${BEAMMEUP_BASE_URL:-https://beammeup.pw}"
INSTALL_DIR="${BEAMMEUP_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${BEAMMEUP_VERSION:-1.4.1}"
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

install_gum_binary() {
  local os_name arch_name latest_tag version tarball url tmp_dir
  local os_raw arch_raw

  os_raw="$(uname -s)"
  arch_raw="$(uname -m)"

  case "$os_raw" in
    Linux) os_name="Linux" ;;
    Darwin) os_name="Darwin" ;;
    *)
      return 1
      ;;
  esac

  case "$arch_raw" in
    x86_64|amd64) arch_name="x86_64" ;;
    arm64|aarch64) arch_name="arm64" ;;
    *)
      return 1
      ;;
  esac

  latest_tag="$(
    curl -fsSL https://api.github.com/repos/charmbracelet/gum/releases/latest \
      | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
      | head -n 1
  )"
  [[ -n "$latest_tag" ]] || return 1
  version="${latest_tag#v}"

  tarball="gum_${version}_${os_name}_${arch_name}.tar.gz"
  url="https://github.com/charmbracelet/gum/releases/download/${latest_tag}/${tarball}"

  tmp_dir="$(mktemp -d -t gum-install.XXXXXX)"
  trap 'rm -rf "$tmp_dir"' RETURN

  curl -fsSL "$url" -o "${tmp_dir}/${tarball}" || return 1
  tar -xzf "${tmp_dir}/${tarball}" -C "$tmp_dir" || return 1

  mkdir -p "$INSTALL_DIR"
  mv "${tmp_dir}/gum" "${INSTALL_DIR}/gum" || return 1
  chmod +x "${INSTALL_DIR}/gum"
  export PATH="${INSTALL_DIR}:$PATH"
  trap - RETURN
  rm -rf "$tmp_dir"
}

install_gum() {
  if command -v gum >/dev/null 2>&1; then
    info "gum already installed"
    return 0
  fi

  info "installing gum for full tui mode..."

  if command -v brew >/dev/null 2>&1; then
    brew install gum >/dev/null 2>&1 || true
  elif command -v apt-get >/dev/null 2>&1; then
    run_with_privilege apt-get update >/dev/null 2>&1 || true
    run_with_privilege apt-get install -y gum >/dev/null 2>&1 || true
  elif command -v dnf >/dev/null 2>&1; then
    run_with_privilege dnf install -y gum >/dev/null 2>&1 || true
  elif command -v yum >/dev/null 2>&1; then
    run_with_privilege yum install -y gum >/dev/null 2>&1 || true
  elif command -v pacman >/dev/null 2>&1; then
    run_with_privilege pacman -Sy --noconfirm gum >/dev/null 2>&1 || true
  elif command -v zypper >/dev/null 2>&1; then
    run_with_privilege zypper --non-interactive install gum >/dev/null 2>&1 || true
  elif command -v apk >/dev/null 2>&1; then
    run_with_privilege apk add gum >/dev/null 2>&1 || true
  fi

  if command -v gum >/dev/null 2>&1; then
    info "gum ready"
    return 0
  fi

  info "package manager install unavailable; trying direct binary install..."
  if install_gum_binary && command -v gum >/dev/null 2>&1; then
    info "gum ready"
    return 0
  fi

  die "failed to install gum automatically. Install manually: https://github.com/charmbracelet/gum"
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

install_gum

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
