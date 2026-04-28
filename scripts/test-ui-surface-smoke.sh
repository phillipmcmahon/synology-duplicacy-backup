#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

assert_contains() {
    file="$1"
    expected="$2"
    if ! grep -F -- "$expected" "$file" >/dev/null; then
        echo "expected $file to contain: $expected" >&2
        exit 1
    fi
}

sh -n "$ROOT/scripts/ui-surface-smoke-runner.sh"
sh -n "$ROOT/scripts/package-ui-surface-smoke.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

sh "$ROOT/scripts/package-ui-surface-smoke.sh" --help >"$tmp/package-help.txt"
assert_contains "$tmp/package-help.txt" "Build a structured NAS UI surface smoke bundle"
assert_contains "$tmp/package-help.txt" "--default-workspace-root"

assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "/usr/local/bin/duplicacy-backup"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "DUPLICACY_BACKUP_FORCE_COLOUR=1"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "ui-surface-captures-"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "RUN_RESTORE=1 requires RESTORE_REVISION and RESTORE_PATH"

echo "ui-surface-smoke script tests passed"
