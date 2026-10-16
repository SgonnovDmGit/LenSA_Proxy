#!/usr/bin/env bash
# build-unix.sh — сборка LenSA Proxy для Linux/macOS (Fyne UI).
# Требует CGO и dev-заголовки fyne (см. README и CI).
# Использование:
#   scripts/build-unix.sh [version]
# Переменные окружения:
#   VERSION       версия (по умолчанию "dev" или первый аргумент)
#   COMMIT        короткий sha (по умолчанию git rev-parse)
#   GOOS_TARGET   linux|darwin (по умолчанию текущая ОС)
#   GOARCH_TARGET amd64|arm64 (по умолчанию текущая архитектура)
#   OUT_DIR       каталог артефактов (по умолчанию dist)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-${ROOT}/dist}"
VERSION="${1:-${VERSION:-dev}}"

case "$(uname -s)" in
  Linux)  HOST_OS="linux" ;;
  Darwin) HOST_OS="darwin" ;;
  *) echo "build-unix.sh: unsupported host OS" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)   HOST_ARCH="amd64" ;;
  arm64|aarch64)  HOST_ARCH="arm64" ;;
  *) echo "build-unix.sh: unsupported host arch $(uname -m)" >&2; exit 1 ;;
esac

GOOS_TARGET="${GOOS_TARGET:-$HOST_OS}"
GOARCH_TARGET="${GOARCH_TARGET:-$HOST_ARCH}"
COMMIT="${COMMIT:-$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}"

mkdir -p "$OUT_DIR"
BINARY="LenSA_Proxy_${GOOS_TARGET}_${GOARCH_TARGET}"
ARCHIVE="${BINARY}.tar.gz"

echo "==> build ${BINARY} (version=${VERSION} commit=${COMMIT})"
CGO_ENABLED=1 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
  go -C "$ROOT" build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
  -o "${OUT_DIR}/${BINARY}" ./cmd/lensa-proxy

tar -czf "${OUT_DIR}/${ARCHIVE}" -C "$OUT_DIR" "$BINARY"
rm -f "${OUT_DIR}/${BINARY}"

if command -v sha256sum >/dev/null 2>&1; then
  HASH=$(sha256sum "${OUT_DIR}/${ARCHIVE}" | cut -d' ' -f1)
else
  HASH=$(shasum -a 256 "${OUT_DIR}/${ARCHIVE}" | cut -d' ' -f1)
fi
echo "==> ${ARCHIVE} sha256=${HASH}"
