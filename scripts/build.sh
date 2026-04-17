#!/usr/bin/env bash
# Build the kcm binary for the current platform.
#
# Usage:
#   scripts/build.sh [output-path]
#
# Examples:
#   scripts/build.sh            # writes to ./bin/kcm
#   scripts/build.sh /tmp/kcm   # writes to /tmp/kcm
set -euo pipefail

cd "$(dirname "$0")/.."

OUT="${1:-./bin/kcm}"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

mkdir -p "$(dirname "$OUT")"

CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags="-s -w \
    -X github.com/loupeznik/kubeconfig-manager/internal/cli.Version=${VERSION} \
    -X github.com/loupeznik/kubeconfig-manager/internal/cli.Commit=${COMMIT} \
    -X github.com/loupeznik/kubeconfig-manager/internal/cli.Date=${DATE}" \
  -o "$OUT" \
  ./cmd/kubeconfig-manager

echo "built: $OUT ($VERSION, $COMMIT)"
