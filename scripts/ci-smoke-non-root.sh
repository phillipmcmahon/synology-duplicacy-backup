#!/bin/sh

set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/ci-smoke-lib.sh"

BINARY="${BINARY:-/tmp/duplicacy-backup}"
OPERATOR_USER="${OPERATOR_USER:-duplicacyci}"
STORAGE="${STORAGE:-onsite-ci}"
IMAGE="${BTRFS_IMAGE:-/tmp/duplicacy-backup-ci-non-root.btrfs}"

cleanup() {
    ci_cleanup_btrfs_volume1 "$IMAGE"
}
trap cleanup EXIT
trap 'cleanup; exit 130' HUP INT TERM

ci_require_root
ci_ensure_dsm_marker
ci_create_operator_user "$OPERATOR_USER"
ci_install_binary "$BINARY"
ci_install_fake_duplicacy
ci_mount_btrfs_volume1 "$IMAGE"
ci_write_local_config "$OPERATOR_USER" "$STORAGE" "/volume1/duplicacy/homes"
chown -R "$OPERATOR_USER:$(id -gn "$OPERATOR_USER")" /volume1/duplicacy

operator_home="$(getent passwd "$OPERATOR_USER" | awk -F: '{print $6; exit}')"

run_operator() {
    sudo -u "$OPERATOR_USER" env -u XDG_CONFIG_HOME -u XDG_STATE_HOME HOME="$operator_home" "$@"
}

assert_output_contains() {
    file="$1"
    expected="$2"
    if ! grep -F -- "$expected" "$file" >/dev/null; then
        cat "$file" >&2
        ci_fail "expected output to contain: $expected"
    fi
}

assert_output_not_matches() {
    file="$1"
    unexpected_pattern="$2"
    if grep -Ei -- "$unexpected_pattern" "$file" >/dev/null; then
        cat "$file" >&2
        ci_fail "expected output not to match: $unexpected_pattern"
    fi
}

run_operator_expect_fail() {
    output_file="$1"
    shift
    if run_operator "$@" >"$output_file" 2>&1; then
        cat "$output_file" >&2
        ci_fail "expected command to fail: $*"
    fi
}

run_operator_expect_policy_fail() {
    output_file="$1"
    expected="$2"
    shift 2
    run_operator_expect_fail "$output_file" "$@"
    if ! grep -F -- "$expected" "$output_file" >/dev/null; then
        cat "$output_file" >&2
        ci_fail "command failed without expected policy output: $expected"
    fi
}

run_operator duplicacy-backup --version
run_operator duplicacy-backup config explain --storage "$STORAGE" homes
run_operator duplicacy-backup config paths --storage "$STORAGE" homes
run_operator duplicacy-backup diagnostics --storage "$STORAGE" homes
run_operator duplicacy-backup restore plan --storage "$STORAGE" homes

chmod 0700 /volume1/duplicacy /volume1/duplicacy/homes
chown -R root:root /volume1/duplicacy

tmp_output="$(mktemp)"
trap 'rm -f "$tmp_output"; cleanup' EXIT

run_operator_expect_policy_fail "$tmp_output" "local filesystem repository is root-protected" duplicacy-backup config validate --storage "$STORAGE" homes
assert_output_contains "$tmp_output" "Storage Access"
assert_output_contains "$tmp_output" "Repository Access"
assert_output_contains "$tmp_output" "Requires sudo"
assert_output_not_matches "$tmp_output" "permission denied|EACCES"

run_operator_expect_policy_fail "$tmp_output" "Requires sudo: local filesystem repository is root-protected" duplicacy-backup health status --storage "$STORAGE" homes
assert_output_contains "$tmp_output" "Repository Access"
assert_output_not_matches "$tmp_output" "permission denied|EACCES"

run_operator_expect_policy_fail "$tmp_output" "restore list-revisions requires sudo: local filesystem repository is root-protected" duplicacy-backup restore list-revisions --storage "$STORAGE" homes
assert_output_not_matches "$tmp_output" "permission denied|EACCES"

run_operator_expect_policy_fail "$tmp_output" "prune --dry-run requires sudo: local filesystem repository is root-protected" duplicacy-backup prune --storage "$STORAGE" --dry-run homes
assert_output_not_matches "$tmp_output" "permission denied|EACCES"
