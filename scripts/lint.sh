#!/usr/bin/env bash
# Severity-aware lint: golangci-lint's exit code cannot distinguish severities
# (severity is reporting metadata), so the run itself never fails on issues
# (--issues-exit-code 0) and this wrapper exits 1 only when an error-severity
# issue is present. Warning-severity findings stay visible on every run
# without failing it; which linters warn is the .golangci.yml severity block.
set -uo pipefail

json="$(mktemp)"
trap 'rm -f "$json"' EXIT

# A non-zero exit here is a tool failure (bad config, compile error), never an
# issue count — propagate it.
golangci-lint run --issues-exit-code 0 \
  --output.text.path stdout --output.json.path "$json" ./... || exit $?

if jq -e 'any(.Issues[]?; .Severity == "error")' "$json" > /dev/null; then
  echo "lint: error-severity issues found" >&2
  exit 1
fi
exit 0
