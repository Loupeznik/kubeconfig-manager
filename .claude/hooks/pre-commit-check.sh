#!/usr/bin/env bash
#
# .claude/hooks/pre-commit-check.sh
#
# Claude Code PreToolUse hook. When the agent invokes a Bash command that
# contains `git commit`, this script runs gofmt, go vet, and go test from the
# repo root. If any check fails, the script exits 2, which blocks the tool
# call and surfaces the failure to the agent so it can fix before retrying.
# For any other Bash command, it exits 0 immediately.
#
# Wired from .claude/settings.json (PreToolUse matcher: "Bash").
#
# Usage: `.claude/hooks/pre-commit-check.sh` — reads the Claude tool-call JSON
# on stdin; no CLI arguments.

set -euo pipefail

payload=$(cat)

# Extract the Bash command. Prefer jq; fall back to python3; else bail out
# silently so a missing optional dep never blocks the agent.
cmd=""
if command -v jq >/dev/null 2>&1; then
	cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // ""' 2>/dev/null || true)
elif command -v python3 >/dev/null 2>&1; then
	cmd=$(printf '%s' "$payload" | python3 -c '
import json, sys
try:
    print(json.load(sys.stdin).get("tool_input", {}).get("command", ""))
except Exception:
    pass
' 2>/dev/null || true)
else
	exit 0
fi

# Only intercept `git commit` invocations. Matching on a whitespace-separated
# boundary avoids false positives on e.g. `git committed` or a variable name.
if ! printf '%s' "$cmd" | grep -Eq '(^|[[:space:]&|;(])git[[:space:]]+commit([[:space:]]|$)'; then
	exit 0
fi

cd "$(git rev-parse --show-toplevel)"

echo "[pre-commit] running gofmt, go vet, go test..." >&2

# gofmt — exclude generated paths that aren't real Go source.
unformatted=$(gofmt -l $(go list -f '{{.Dir}}' ./... 2>/dev/null) 2>/dev/null || true)
if [ -n "$unformatted" ]; then
	echo "[pre-commit] gofmt would reformat:" >&2
	echo "$unformatted" >&2
	echo "[pre-commit] run 'gofmt -w .' and retry" >&2
	exit 2
fi

if ! go vet ./... 1>&2; then
	echo "[pre-commit] go vet failed" >&2
	exit 2
fi

if ! go test ./... 1>&2; then
	echo "[pre-commit] go test failed" >&2
	exit 2
fi

# Optional: golangci-lint, skipped if not installed. CI enforces it regardless.
if command -v golangci-lint >/dev/null 2>&1; then
	if ! golangci-lint run ./... 1>&2; then
		echo "[pre-commit] golangci-lint failed" >&2
		exit 2
	fi
else
	echo "[pre-commit] golangci-lint not installed, skipping (install via brew/binary to enforce locally)" >&2
fi

# Optional: goreleaser config sanity, skipped if not installed.
if command -v goreleaser >/dev/null 2>&1 && [ -f .goreleaser.yaml ]; then
	if ! goreleaser check 1>&2; then
		echo "[pre-commit] goreleaser check failed" >&2
		exit 2
	fi
else
	echo "[pre-commit] goreleaser not installed, skipping (CI covers this regardless)" >&2
fi

echo "[pre-commit] all checks passed" >&2
exit 0
