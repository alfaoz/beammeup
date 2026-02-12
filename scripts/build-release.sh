#!/usr/bin/env bash
set -euo pipefail

# Build beammeup release archives for supported platforms.
# Output files:
#   dist/beammeup_<os>_<arch>.tar.gz

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/dist}"
VERSION="${VERSION:-}"

mkdir -p "$OUT_DIR"

if [[ -z "$VERSION" ]]; then
  VERSION="$(git -C "$ROOT_DIR" describe --tags --always 2>/dev/null | sed 's/^v//')"
fi

echo "[build] version: ${VERSION}"
echo "[build] output: ${OUT_DIR}"

platforms=(
  "darwin arm64"
  "darwin amd64"
  "linux amd64"
  "linux arm64"
)

for entry in "${platforms[@]}"; do
  os="${entry%% *}"
  arch="${entry##* }"
  work="${OUT_DIR}/build_${os}_${arch}"
  mkdir -p "$work"

  echo "[build] ${os}/${arch}"
  (cd "$ROOT_DIR" && \
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X github.com/alfaoz/beammeup/internal/version.AppVersion=${VERSION}" \
    -o "${work}/beammeup" ./cmd/beammeup)

  tar -C "$work" -czf "${OUT_DIR}/beammeup_${os}_${arch}.tar.gz" beammeup
  rm -rf "$work"
done

printf '%s\n' "${VERSION}" > "${OUT_DIR}/version.txt"
echo "[build] done"
