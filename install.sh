#!/usr/bin/env bash
set -euo pipefail

# beammeup installer (binary release)
#
# Usage:
#   curl -fsSL https://beammeup.pw/install.sh | bash
#
# Optional env vars:
#   BEAMMEUP_REPO        (default: alfaoz/beammeup)
#   BEAMMEUP_BASE_URL    (default: empty -> GitHub releases)
#   BEAMMEUP_INSTALL_DIR (default: $HOME/.local/bin)
#   BEAMMEUP_VERSION     (default: latest)

REPO="${BEAMMEUP_REPO:-alfaoz/beammeup}"
BASE_URL="${BEAMMEUP_BASE_URL:-}"
INSTALL_DIR="${BEAMMEUP_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${BEAMMEUP_VERSION:-latest}"
TMP_DIR=""

info() {
  printf '[beammedown] %s\n' "$*"
}

die() {
  printf '[beammedown] ERROR: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

cleanup() {
  if [[ -n "${TMP_DIR:-}" && -d "${TMP_DIR:-}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}

sha256_file() {
  local path="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{print $1}'
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$path" | awk '{print $NF}'
    return
  fi
  die "sha256sum, shasum, or openssl is required for checksum verification"
}

verify_sha256() {
  local sums_file="$1"
  local asset="$2"
  local archive_path="$3"

  local expected actual
  expected="$(
    awk -v a="$asset" '{
      h=$1; f=$2;
      gsub(/^\*/, "", f);
      sub(/^\.\//, "", f);
      if (f==a || f=="dist/"a) { print h; exit }
    }' "$sums_file"
  )"
  [[ -n "$expected" ]] || die "SHA256SUMS missing entry for ${asset}"

  actual="$(sha256_file "$archive_path")"
  [[ "$actual" == "$expected" ]] || die "checksum mismatch for ${asset} (expected ${expected}, got ${actual})"
}

detect_platform() {
  local os_raw arch_raw
  os_raw="$(uname -s)"
  arch_raw="$(uname -m)"

  case "$os_raw" in
    Darwin) OS="darwin" ;;
    Linux) OS="linux" ;;
    *) die "unsupported OS: $os_raw (supported: Darwin, Linux)" ;;
  esac

  case "$arch_raw" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) die "unsupported architecture: $arch_raw (supported: amd64, arm64)" ;;
  esac
}

normalize_tag() {
  local v="$1"
  if [[ "$v" == latest ]]; then
    printf 'latest'
  elif [[ "$v" == v* ]]; then
    printf '%s' "$v"
  else
    printf 'v%s' "$v"
  fi
}

resolve_download_url() {
  local asset="$1"
  local tag

  if [[ -n "$BASE_URL" ]]; then
    local base
    base="${BASE_URL%/}"
    if [[ "$VERSION" == latest ]]; then
      DOWNLOAD_URL="${base}/releases/latest/${asset}"
      SUMS_URL="${base}/releases/latest/SHA256SUMS"
      if version_txt="$(curl -fsSL "${base}/releases/latest/version.txt" 2>/dev/null)"; then
        DISPLAY_VERSION="${version_txt#v}"
      else
        DISPLAY_VERSION="latest"
      fi
    else
      tag="$(normalize_tag "$VERSION")"
      DOWNLOAD_URL="${base}/releases/download/${tag}/${asset}"
      SUMS_URL="${base}/releases/download/${tag}/SHA256SUMS"
      DISPLAY_VERSION="${tag#v}"
    fi
    return
  fi

  if [[ "$VERSION" == latest ]]; then
    DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${asset}"
    SUMS_URL="https://github.com/${REPO}/releases/latest/download/SHA256SUMS"
    DISPLAY_VERSION="latest"
    return
  fi

  tag="$(normalize_tag "$VERSION")"
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${tag}/${asset}"
  SUMS_URL="https://github.com/${REPO}/releases/download/${tag}/SHA256SUMS"
  DISPLAY_VERSION="${tag#v}"
}

main() {
  need_cmd curl
  need_cmd tar
  need_cmd chmod

  detect_platform

  local asset archive_path sums_path target_path
  asset="beammeup_${OS}_${ARCH}.tar.gz"
  resolve_download_url "$asset"

  target_path="${INSTALL_DIR}/beammeup"
  TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t beammeup-install)"
  trap cleanup EXIT
  archive_path="${TMP_DIR}/${asset}"
  sums_path="${TMP_DIR}/SHA256SUMS"

  if [[ "${DISPLAY_VERSION}" == "latest" ]]; then
    info "beaming down beammeup (latest)"
  else
    info "beaming down beammeup v${DISPLAY_VERSION}"
  fi
  info "downloading ${DOWNLOAD_URL}"

  if ! curl -fsSL "$DOWNLOAD_URL" -o "$archive_path"; then
    die "download failed (${DOWNLOAD_URL})"
  fi

  info "verifying checksum (${SUMS_URL})"
  if ! curl -fsSL "$SUMS_URL" -o "$sums_path"; then
    die "failed to download SHA256SUMS (${SUMS_URL})"
  fi
  verify_sha256 "$sums_path" "$asset" "$archive_path"

  if ! tar -xzf "$archive_path" -C "$TMP_DIR"; then
    die "failed to extract archive"
  fi

  if [[ ! -f "${TMP_DIR}/beammeup" ]]; then
    die "archive did not contain beammeup binary"
  fi

  mkdir -p "$INSTALL_DIR"
  mv "${TMP_DIR}/beammeup" "$target_path"
  chmod 755 "$target_path"

  info "installed to ${target_path}"

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
}

main "$@"
