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
assert_contains "$tmp/package-help.txt" "--default-restore-path"
assert_contains "$tmp/package-help.txt" "--default-run-restore"
assert_contains "$tmp/package-help.txt" "--default-restore-use-sudo"
assert_contains "$tmp/package-help.txt" "--default-restore-targets"
assert_contains "$tmp/package-help.txt" "--default-restore-use-sudo-targets"

assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "/usr/local/bin/duplicacy-backup"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "DUPLICACY_BACKUP_FORCE_COLOUR=1"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "sudo -n DUPLICACY_BACKUP_FORCE_COLOUR=1"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "ui-surface-captures-"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "RESTORE_USE_SUDO"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "RESTORE_TARGETS"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "RESTORE_USE_SUDO_TARGETS"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "SMOKE_SUDO_BIN"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "/usr/local/lib/duplicacy-backup/smoke/duplicacy-backup-smoke"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "validate_smoke_sudo_bin"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "sudo -n install -d"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "sudo -n install -o root -g root -m 0755"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" 'sudo -n "$SMOKE_SUDO_BIN"'
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "run_restore_capture"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "RESTORE_REVISION is auto-selected when omitted"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "restore_revision_auto_select"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "restore_revision_lookup"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "extract_first_revision"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "build_smoke_restore_root"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "run_restore_template_matrix"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "restore_workspace_template_dry_run"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "restore_workspace_root_prepare"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "{label}-rev{revision}-{target}-{run_timestamp}"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "restore_run_optional"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_contains \"-ignore-owner\""
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_contains \"-smoke-\""
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_not_matches"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "capture_has_semantic_value"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_value_colour"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_validation_success_colours"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_validation_warning_colours"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_validation_failure_colours"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "assert_last_capture_report_semantic_colours"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "Available\" green"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "Present\" green"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "Requires sudo\" yellow"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "Unreadable (\" red"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "Failed\" red"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" 'CAPTURE_COLOUR="${CAPTURE_COLOUR:-1}"'
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "local filesystem repository is root-protected"
assert_contains "$ROOT/scripts/ui-surface-smoke-runner.sh" "permission denied|EACCES"
assert_contains "$ROOT/scripts/package-ui-surface-smoke.sh" "SMOKE_SUDO_BIN"
assert_contains "$ROOT/docs/ui-surface-smoke.md" "Sudo Policy for Smoke Runs"
assert_contains "$ROOT/docs/ui-surface-smoke.md" "DUPLICACY_BACKUP_SMOKE_INSTALL"
assert_contains "$ROOT/docs/ui-surface-smoke.md" "/var/services/homes/<operator>/exclude/testing"
assert_contains "$ROOT/docs/ui-surface-smoke.md" "NOPASSWD: SETENV: DUPLICACY_BACKUP_SMOKE"

echo "ui-surface-smoke script tests passed"
