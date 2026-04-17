#!/usr/bin/env bash
# Run formatting, vet, and golangci-lint (if installed) against the module.
#
# Usage:
#   scripts/lint.sh
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> gofmt"
unformatted="$(gofmt -l . 2>/dev/null | grep -v '^vendor/' || true)"
if [[ -n "$unformatted" ]]; then
  echo "the following files need gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> go vet"
go vet ./...

if command -v golangci-lint >/dev/null 2>&1; then
  echo "==> golangci-lint"
  golangci-lint run ./...
else
  echo "skipping golangci-lint (not installed)"
fi
