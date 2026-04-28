#!/bin/sh

set -u

script_dir() {
    CDPATH= cd -- "$(dirname -- "$0")" && pwd
}

safe_name() {
    printf '%s' "$1" | sed 's/[^A-Za-z0-9._-]/_/g; s/__*/_/g; s/^_//; s/_$//'
}

quote_arg() {
    printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

format_command() {
    first=1
    for arg in "$@"; do
        if [ "$first" -eq 1 ]; then
            first=0
        else
            printf ' '
        fi
        quote_arg "$arg"
    done
}

resolve_managed_bin() {
    candidate="${MANAGED_BIN:-}"
    if [ -n "$candidate" ]; then
        case "$candidate" in
            */*)
                if [ -x "$candidate" ]; then
                    printf '%s\n' "$candidate"
                    return 0
                fi
                return 1
                ;;
            *)
                if command -v "$candidate" >/dev/null 2>&1; then
                    command -v "$candidate"
                    return 0
                fi
                ;;
        esac
    fi

    if [ -x /usr/local/bin/duplicacy-backup ]; then
        printf '%s\n' /usr/local/bin/duplicacy-backup
        return 0
    fi

    if command -v duplicacy-backup >/dev/null 2>&1; then
        command -v duplicacy-backup
        return 0
    fi

    return 1
}

run_capture() {
    name="$1"
    expectation="$2"
    shift 2

    index=$((index + 1))
    file_name="$(printf '%02d_%s.txt' "$index" "$(safe_name "$name")")"
    output_file="$CAPTURE_DIR/$file_name"
    command_text="$(format_command "$@")"

    {
        printf 'Name: %s\n' "$name"
        printf 'Expectation: %s\n' "$expectation"
        printf 'Command: %s\n' "$command_text"
        printf '%s\n\n' '--- output ---'
    } > "$output_file"

    if [ "${CAPTURE_COLOUR:-0}" = "1" ]; then
        if [ "${1:-}" = "sudo" ] && [ "${2:-}" = "-n" ]; then
            shift 2
            sudo -n env DUPLICACY_BACKUP_FORCE_COLOUR=1 "$@" >> "$output_file" 2>&1
        else
            DUPLICACY_BACKUP_FORCE_COLOUR=1 "$@" >> "$output_file" 2>&1
        fi
    else
        "$@" >> "$output_file" 2>&1
    fi
    code=$?

    {
        printf '\n%s\n' '--- result ---'
        printf 'Exit code: %s\n' "$code"
    } >> "$output_file"

    case "$expectation" in
        pass)
            if [ "$code" -eq 0 ]; then
                status="PASS"
            else
                status="UNEXPECTED_FAIL"
                unexpected_count=$((unexpected_count + 1))
            fi
            ;;
        fail)
            if [ "$code" -ne 0 ]; then
                status="EXPECTED_FAIL"
            else
                status="UNEXPECTED_PASS"
                unexpected_count=$((unexpected_count + 1))
            fi
            ;;
        any)
            status="CAPTURED"
            ;;
        *)
            status="CAPTURED"
            ;;
    esac

    printf '%s\t%s\t%s\t%s\t%s\n' "$index" "$name" "$status" "$code" "$file_name" >> "$SUMMARY"
    printf '%-50s %s (exit %s)\n' "$name" "$status" "$code"
}

skip_capture() {
    name="$1"
    reason="$2"

    index=$((index + 1))
    file_name="$(printf '%02d_%s.txt' "$index" "$(safe_name "$name")")"
    output_file="$CAPTURE_DIR/$file_name"

    {
        printf 'Name: %s\n' "$name"
        printf 'Status: SKIPPED\n'
        printf 'Reason: %s\n' "$reason"
    } > "$output_file"

    printf '%s\t%s\tSKIPPED\t-\t%s\n' "$index" "$name" "$file_name" >> "$SUMMARY"
    printf '%-50s SKIPPED\n' "$name"
}

assert_last_capture_contains() {
    expected="$1"
    if ! grep -F -- "$expected" "$output_file" >/dev/null; then
        echo "Expected latest capture to contain: $expected" >&2
        echo "Capture file: $output_file" >&2
        unexpected_count=$((unexpected_count + 1))
    fi
}

assert_last_capture_not_matches() {
    unexpected_pattern="$1"
    if grep -Ei -- "$unexpected_pattern" "$output_file" >/dev/null; then
        echo "Expected latest capture not to match: $unexpected_pattern" >&2
        echo "Capture file: $output_file" >&2
        unexpected_count=$((unexpected_count + 1))
    fi
}

extract_first_revision() {
    file="$1"
    awk -F: '
        /^[[:space:]]*Revision[[:space:]]*:/ {
            value = $2
            sub(/^[[:space:]]*/, "", value)
            split(value, parts, /[[:space:](]/)
            if (parts[1] ~ /^[0-9]+$/) {
                print parts[1]
                exit
            }
        }
    ' "$file"
}

extract_revision_timestamp() {
    file="$1"
    revision="$2"
    # Soft dependency on the human restore list-revisions output. If the
    # format changes, the smoke workspace still works with unknown-snapshot.
    awk -v rev="$revision" '
        /^[[:space:]]*Revision[[:space:]]*:/ {
            line = $0
            sub(/^[[:space:]]*Revision[[:space:]]*:[[:space:]]*/, "", line)
            split(line, parts, /[[:space:](]/)
            if (parts[1] == rev) {
                split($0, left, "(")
                if (left[2] != "") {
                    split(left[2], right, ")")
                    print right[1]
                    exit
                }
            }
        }
    ' "$file"
}

format_snapshot_timestamp() {
    value="$1"
    if [ -z "$value" ]; then
        printf '%s\n' "unknown-snapshot"
        return 0
    fi
    printf '%s' "$value" | sed 's/-//g; s/://g; s/ /-/g'
}

read_build_commit() {
    file="$BUNDLE_ROOT/metadata/build.json"
    [ -f "$file" ] || return 1
    sed -n 's/.*"git_commit"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$file" | sed -n '1p'
}

short_build_commit() {
    commit="$(read_build_commit 2>/dev/null || true)"
    case "$commit" in
        ""|unknown)
            # Bundles should have build.json. This fallback only covers ad-hoc
            # git-describe-style binaries such as v9.1.6-1-gabcdef0.
            commit="$("$BIN" --version 2>/dev/null | sed -n 's/.*-g\([0-9a-f][0-9a-f]*\).*/\1/p' | sed -n '1p')"
            ;;
    esac
    if [ -z "$commit" ]; then
        printf '%s\n' "unknown"
        return 0
    fi
    printf '%s\n' "$commit" | cut -c1-7
}

build_smoke_restore_workspace() {
    snapshot_timestamp="$1"
    short_commit="$2"
    run_timestamp="$3"
    snapshot_part="$(format_snapshot_timestamp "$snapshot_timestamp")"
    printf '%s/%s-%s-%s-rev%s-smoke-%s-%s\n' \
        "$WORKSPACE_ROOT" \
        "$(safe_name "$LABEL")" \
        "$(safe_name "$RESTORE_TARGET")" \
        "$(safe_name "$snapshot_part")" \
        "$(safe_name "$RESTORE_REVISION")" \
        "$(safe_name "$short_commit")" \
        "$(safe_name "$run_timestamp")"
}

BUNDLE_ROOT="$(script_dir)"
# shellcheck disable=SC1091
. "$BUNDLE_ROOT/setup-env.sh" >/dev/null

LABEL="${LABEL:-homes}"
TARGET_REMOTE="${TARGET_REMOTE:-offsite-storj}"
TARGET_OBJECT="${TARGET_OBJECT:-onsite-garage}"
TARGET_LOCAL="${TARGET_LOCAL:-onsite-usb}"
WORKSPACE_ROOT="${WORKSPACE_ROOT:-/volume1/restore-drills}"
RUN_NOTIFY="${RUN_NOTIFY:-0}"
RUN_RESTORE="${RUN_RESTORE:-0}"
CAPTURE_COLOUR="${CAPTURE_COLOUR:-0}"
RESTORE_TARGET="${RESTORE_TARGET:-$TARGET_REMOTE}"
RESTORE_REVISION="${RESTORE_REVISION:-}"
RESTORE_PATH="${RESTORE_PATH:-}"
RESTORE_WORKSPACE="${RESTORE_WORKSPACE:-}"
RESTORE_REVISION_LOOKUP_LIMIT="${RESTORE_REVISION_LOOKUP_LIMIT:-200}"

if MANAGED_BIN_RESOLVED="$(resolve_managed_bin)"; then
    managed_status="$MANAGED_BIN_RESOLVED"
else
    MANAGED_BIN_RESOLVED=""
    managed_status="not found"
fi

STAMP="$(date -u '+%Y%m%d%H%M%S')"
CAPTURE_DIR="${CAPTURE_DIR:-$BUNDLE_ROOT/captures/$STAMP}"
mkdir -p "$CAPTURE_DIR"

SUMMARY="$CAPTURE_DIR/summary.tsv"
index=0
unexpected_count=0

{
    printf 'Bundle\t%s\n' "$BUNDLE_ROOT"
    printf 'Binary\t%s\n' "$BIN"
    printf 'Label\t%s\n' "$LABEL"
    printf 'Remote target\t%s\n' "$TARGET_REMOTE"
    printf 'Object target\t%s\n' "$TARGET_OBJECT"
    printf 'Local target\t%s\n' "$TARGET_LOCAL"
    printf 'Workspace root\t%s\n' "$WORKSPACE_ROOT"
    printf 'Managed binary\t%s\n' "$managed_status"
    printf 'Capture colour\t%s\n' "$CAPTURE_COLOUR"
    printf '\n'
    printf 'Index\tName\tStatus\tExitCode\tFile\n'
} > "$SUMMARY"

cat <<EOF
UI surface smoke capture
  Binary        : $BIN
  Label         : $LABEL
  Remote target : $TARGET_REMOTE
  Object target : $TARGET_OBJECT
  Local target  : $TARGET_LOCAL
  Managed binary: $managed_status
  Capture colour: $CAPTURE_COLOUR
  Capture dir   : $CAPTURE_DIR

EOF

run_capture "meta_version" pass "$BIN" --version
run_capture "help_short" pass "$BIN" --help
run_capture "help_full" pass "$BIN" --help-full

run_capture "config_explain_remote" any "$BIN" config explain --target "$TARGET_REMOTE" "$LABEL"
run_capture "config_explain_object" any "$BIN" config explain --target "$TARGET_OBJECT" "$LABEL"
run_capture "config_explain_local" any "$BIN" config explain --target "$TARGET_LOCAL" "$LABEL"
run_capture "config_paths_remote" any "$BIN" config paths --target "$TARGET_REMOTE" "$LABEL"
run_capture "config_validate_remote" any "$BIN" config validate --target "$TARGET_REMOTE" "$LABEL"
run_capture "config_validate_object" any "$BIN" config validate --target "$TARGET_OBJECT" "$LABEL"
run_capture "config_validate_local_operator_requires_sudo" fail "$BIN" config validate --target "$TARGET_LOCAL" "$LABEL"
assert_last_capture_contains "Storage Access"
assert_last_capture_contains "Repository Access"
assert_last_capture_contains "Requires sudo"
assert_last_capture_contains "local filesystem repository is root-protected"
assert_last_capture_not_matches "permission denied|EACCES"
run_capture "config_validate_local_sudo" any sudo -n "$BIN" config validate --target "$TARGET_LOCAL" "$LABEL"
run_capture "config_validate_remote_verbose" any "$BIN" config validate --target "$TARGET_REMOTE" --verbose "$LABEL"
run_capture "config_validate_local_sudo_verbose" any sudo -n "$BIN" config validate --target "$TARGET_LOCAL" --verbose "$LABEL"

run_capture "diagnostics_remote" any "$BIN" diagnostics --target "$TARGET_REMOTE" "$LABEL"
run_capture "diagnostics_object" any "$BIN" diagnostics --target "$TARGET_OBJECT" "$LABEL"
run_capture "diagnostics_local_operator" any "$BIN" diagnostics --target "$TARGET_LOCAL" "$LABEL"
run_capture "diagnostics_local_sudo" any sudo -n "$BIN" diagnostics --target "$TARGET_LOCAL" "$LABEL"
run_capture "diagnostics_remote_json_summary" any "$BIN" diagnostics --target "$TARGET_REMOTE" --json-summary "$LABEL"
run_capture "diagnostics_local_sudo_json_summary" any sudo -n "$BIN" diagnostics --target "$TARGET_LOCAL" --json-summary "$LABEL"

run_capture "health_status_remote" any "$BIN" health status --target "$TARGET_REMOTE" "$LABEL"
run_capture "health_doctor_remote" any "$BIN" health doctor --target "$TARGET_REMOTE" "$LABEL"
run_capture "health_verify_remote" any "$BIN" health verify --target "$TARGET_REMOTE" "$LABEL"
run_capture "health_status_object" any "$BIN" health status --target "$TARGET_OBJECT" "$LABEL"
run_capture "health_doctor_object" any "$BIN" health doctor --target "$TARGET_OBJECT" "$LABEL"
run_capture "health_verify_object" any "$BIN" health verify --target "$TARGET_OBJECT" "$LABEL"
run_capture "health_status_local_operator_requires_sudo" fail "$BIN" health status --target "$TARGET_LOCAL" "$LABEL"
assert_last_capture_contains "Repository Access"
assert_last_capture_contains "Requires sudo: local filesystem repository is root-protected"
assert_last_capture_not_matches "permission denied|EACCES"
run_capture "health_doctor_local_operator_requires_sudo" fail "$BIN" health doctor --target "$TARGET_LOCAL" "$LABEL"
assert_last_capture_contains "Repository Access"
assert_last_capture_contains "Requires sudo: local filesystem repository is root-protected"
assert_last_capture_not_matches "permission denied|EACCES"
run_capture "health_verify_local_operator_requires_sudo" fail "$BIN" health verify --target "$TARGET_LOCAL" "$LABEL"
assert_last_capture_contains "Repository Access"
assert_last_capture_contains "Requires sudo: local filesystem repository is root-protected"
assert_last_capture_not_matches "permission denied|EACCES"
run_capture "health_status_local_sudo" any sudo -n "$BIN" health status --target "$TARGET_LOCAL" "$LABEL"
run_capture "health_doctor_local_sudo" any sudo -n "$BIN" health doctor --target "$TARGET_LOCAL" "$LABEL"
run_capture "health_verify_local_sudo" any sudo -n "$BIN" health verify --target "$TARGET_LOCAL" "$LABEL"
run_capture "health_status_remote_json_summary" any "$BIN" health status --target "$TARGET_REMOTE" --json-summary "$LABEL"
run_capture "health_doctor_remote_verbose" any "$BIN" health doctor --target "$TARGET_REMOTE" --verbose "$LABEL"
run_capture "health_doctor_remote_verbose_json_summary" any "$BIN" health doctor --target "$TARGET_REMOTE" --verbose --json-summary "$LABEL"
run_capture "health_status_local_sudo_json_summary" any sudo -n "$BIN" health status --target "$TARGET_LOCAL" --json-summary "$LABEL"
run_capture "health_doctor_local_sudo_verbose" any sudo -n "$BIN" health doctor --target "$TARGET_LOCAL" --verbose "$LABEL"
run_capture "health_doctor_local_sudo_verbose_json_summary" any sudo -n "$BIN" health doctor --target "$TARGET_LOCAL" --verbose --json-summary "$LABEL"

run_capture "restore_plan_remote" any "$BIN" restore plan --target "$TARGET_REMOTE" "$LABEL"
run_capture "restore_list_revisions_remote" any "$BIN" restore list-revisions --target "$TARGET_REMOTE" --limit 5 "$LABEL"
run_capture "restore_plan_object" any "$BIN" restore plan --target "$TARGET_OBJECT" "$LABEL"
run_capture "restore_list_revisions_object" any "$BIN" restore list-revisions --target "$TARGET_OBJECT" --limit 5 "$LABEL"
run_capture "restore_plan_local_operator" any "$BIN" restore plan --target "$TARGET_LOCAL" "$LABEL"
run_capture "restore_list_revisions_local_operator_requires_sudo" fail "$BIN" restore list-revisions --target "$TARGET_LOCAL" --limit 5 "$LABEL"
assert_last_capture_contains "restore list-revisions requires sudo: local filesystem repository is root-protected"
assert_last_capture_not_matches "permission denied|EACCES"
run_capture "restore_list_revisions_local_sudo" any sudo -n "$BIN" restore list-revisions --target "$TARGET_LOCAL" --limit 5 "$LABEL"
run_capture "restore_list_revisions_remote_json_summary" any "$BIN" restore list-revisions --target "$TARGET_REMOTE" --limit 5 --json-summary "$LABEL"
run_capture "restore_list_revisions_local_sudo_json_summary" any sudo -n "$BIN" restore list-revisions --target "$TARGET_LOCAL" --limit 5 --json-summary "$LABEL"

if [ "$RUN_RESTORE" = "1" ]; then
    if [ -n "$RESTORE_PATH" ]; then
        revision_listing_file=""
        if [ -z "$RESTORE_REVISION" ]; then
            run_capture "restore_revision_auto_select" pass "$BIN" restore list-revisions --target "$RESTORE_TARGET" --limit 1 "$LABEL"
            revision_listing_file="$output_file"
            RESTORE_REVISION="$(extract_first_revision "$output_file")"
            if [ -z "$RESTORE_REVISION" ]; then
                echo "Unable to auto-select restore revision from: $output_file" >&2
                unexpected_count=$((unexpected_count + 1))
            else
                printf '%-50s %s\n' "restore_revision_auto_select_value" "$RESTORE_REVISION"
            fi
        else
            run_capture "restore_revision_lookup" any "$BIN" restore list-revisions --target "$RESTORE_TARGET" --limit "$RESTORE_REVISION_LOOKUP_LIMIT" "$LABEL"
            revision_listing_file="$output_file"
        fi
        if [ -n "$RESTORE_REVISION" ]; then
            if [ -z "$RESTORE_WORKSPACE" ]; then
                snapshot_timestamp="$(extract_revision_timestamp "$revision_listing_file" "$RESTORE_REVISION")"
                RESTORE_WORKSPACE="$(build_smoke_restore_workspace "$snapshot_timestamp" "$(short_build_commit)" "$STAMP")"
            fi
            printf '%-50s %s\n' "restore_workspace_smoke" "$RESTORE_WORKSPACE"
            run_capture "restore_run_optional" pass "$BIN" restore run --target "$RESTORE_TARGET" --revision "$RESTORE_REVISION" --path "$RESTORE_PATH" --workspace "$RESTORE_WORKSPACE" --yes "$LABEL"
            assert_last_capture_contains "-ignore-owner"
            assert_last_capture_contains "-smoke-"
            run_capture "restore_run_optional_dry_run" pass "$BIN" restore run --target "$RESTORE_TARGET" --revision "$RESTORE_REVISION" --path "$RESTORE_PATH" --workspace "$RESTORE_WORKSPACE" --dry-run --yes "$LABEL"
            assert_last_capture_contains "-ignore-owner"
            assert_last_capture_contains "-smoke-"
        else
            skip_capture "restore_run_optional" "Restore revision auto-selection failed"
            skip_capture "restore_run_optional_dry_run" "Restore revision auto-selection failed"
        fi
    else
        skip_capture "restore_run_optional" "RUN_RESTORE=1 requires RESTORE_PATH; RESTORE_REVISION is auto-selected when omitted"
        skip_capture "restore_run_optional_dry_run" "RUN_RESTORE=1 requires RESTORE_PATH; RESTORE_REVISION is auto-selected when omitted"
        unexpected_count=$((unexpected_count + 1))
    fi
else
    skip_capture "restore_run_optional" "Actual restore is skipped by default; set RUN_RESTORE=1 with RESTORE_PATH"
    skip_capture "restore_run_optional_dry_run" "Restore dry-run is skipped by default; set RUN_RESTORE=1 with RESTORE_PATH"
fi
skip_capture "restore_select_interactive" "Interactive TUI; run manually with tee as described in instructions/smoke-test.md"

run_capture "backup_dry_run_remote_sudo" any sudo -n "$BIN" backup --target "$TARGET_REMOTE" --dry-run "$LABEL"
run_capture "backup_dry_run_object_sudo" any sudo -n "$BIN" backup --target "$TARGET_OBJECT" --dry-run "$LABEL"
run_capture "backup_dry_run_local_sudo" any sudo -n "$BIN" backup --target "$TARGET_LOCAL" --dry-run "$LABEL"
run_capture "backup_dry_run_remote_sudo_verbose" any sudo -n "$BIN" backup --target "$TARGET_REMOTE" --dry-run --verbose "$LABEL"
run_capture "backup_dry_run_remote_sudo_json_summary" any sudo -n "$BIN" backup --target "$TARGET_REMOTE" --dry-run --json-summary "$LABEL"
run_capture "backup_dry_run_remote_sudo_verbose_json_summary" any sudo -n "$BIN" backup --target "$TARGET_REMOTE" --dry-run --verbose --json-summary "$LABEL"
run_capture "backup_dry_run_object_sudo_verbose_json_summary" any sudo -n "$BIN" backup --target "$TARGET_OBJECT" --dry-run --verbose --json-summary "$LABEL"
run_capture "backup_dry_run_local_sudo_verbose_json_summary" any sudo -n "$BIN" backup --target "$TARGET_LOCAL" --dry-run --verbose --json-summary "$LABEL"

run_capture "prune_dry_run_remote_operator" any "$BIN" prune --target "$TARGET_REMOTE" --dry-run "$LABEL"
run_capture "prune_dry_run_object_operator" any "$BIN" prune --target "$TARGET_OBJECT" --dry-run "$LABEL"
run_capture "prune_dry_run_local_operator_requires_sudo" fail "$BIN" prune --target "$TARGET_LOCAL" --dry-run "$LABEL"
assert_last_capture_contains "prune --dry-run requires sudo: local filesystem repository is root-protected"
assert_last_capture_not_matches "permission denied|EACCES"
run_capture "prune_dry_run_local_sudo" any sudo -n "$BIN" prune --target "$TARGET_LOCAL" --dry-run "$LABEL"
run_capture "prune_dry_run_remote_operator_verbose" any "$BIN" prune --target "$TARGET_REMOTE" --dry-run --verbose "$LABEL"
run_capture "prune_dry_run_remote_operator_json_summary" any "$BIN" prune --target "$TARGET_REMOTE" --dry-run --json-summary "$LABEL"
run_capture "prune_dry_run_remote_operator_verbose_json_summary" any "$BIN" prune --target "$TARGET_REMOTE" --dry-run --verbose --json-summary "$LABEL"
run_capture "prune_dry_run_local_sudo_verbose_json_summary" any sudo -n "$BIN" prune --target "$TARGET_LOCAL" --dry-run --verbose --json-summary "$LABEL"

run_capture "cleanup_storage_dry_run_remote_operator" any "$BIN" cleanup-storage --target "$TARGET_REMOTE" --dry-run "$LABEL"
run_capture "cleanup_storage_dry_run_object_operator" any "$BIN" cleanup-storage --target "$TARGET_OBJECT" --dry-run "$LABEL"
run_capture "cleanup_storage_dry_run_local_operator" any "$BIN" cleanup-storage --target "$TARGET_LOCAL" --dry-run "$LABEL"
run_capture "cleanup_storage_dry_run_local_sudo" any sudo -n "$BIN" cleanup-storage --target "$TARGET_LOCAL" --dry-run "$LABEL"
run_capture "cleanup_storage_dry_run_remote_operator_verbose" any "$BIN" cleanup-storage --target "$TARGET_REMOTE" --dry-run --verbose "$LABEL"
run_capture "cleanup_storage_dry_run_remote_operator_json_summary" any "$BIN" cleanup-storage --target "$TARGET_REMOTE" --dry-run --json-summary "$LABEL"
run_capture "cleanup_storage_dry_run_remote_operator_verbose_json_summary" any "$BIN" cleanup-storage --target "$TARGET_REMOTE" --dry-run --verbose --json-summary "$LABEL"
run_capture "cleanup_storage_dry_run_local_sudo_verbose_json_summary" any sudo -n "$BIN" cleanup-storage --target "$TARGET_LOCAL" --dry-run --verbose --json-summary "$LABEL"

run_capture "notify_test_backup_remote_dry_run" any "$BIN" notify test --target "$TARGET_REMOTE" --dry-run "$LABEL"
run_capture "notify_test_backup_remote_dry_run_json_summary" any "$BIN" notify test --target "$TARGET_REMOTE" --dry-run --json-summary "$LABEL"
run_capture "notify_test_update_dry_run" any "$BIN" notify test update --dry-run
run_capture "notify_test_update_dry_run_json_summary" any "$BIN" notify test update --dry-run --json-summary

if [ "$RUN_NOTIFY" = "1" ]; then
    run_capture "notify_test_backup_remote" any "$BIN" notify test --target "$TARGET_REMOTE" "$LABEL"
    run_capture "notify_test_update" any "$BIN" notify test update
else
    skip_capture "notify_test_backup_remote" "Notifications are skipped by default; set RUN_NOTIFY=1 to send test notifications"
    skip_capture "notify_test_update" "Notifications are skipped by default; set RUN_NOTIFY=1 to send test notifications"
fi

run_capture "update_check_only_smoke_binary" any "$BIN" update --check-only
run_capture "rollback_check_only_smoke_binary" any "$BIN" rollback --check-only
if [ -n "$MANAGED_BIN_RESOLVED" ]; then
    run_capture "update_check_only_managed_command" any "$MANAGED_BIN_RESOLVED" update --check-only
    run_capture "rollback_check_only_managed_command" any "$MANAGED_BIN_RESOLVED" rollback --check-only
else
    skip_capture "update_check_only_managed_command" "Managed command not found; set MANAGED_BIN=/usr/local/bin/duplicacy-backup"
    skip_capture "rollback_check_only_managed_command" "Managed command not found; set MANAGED_BIN=/usr/local/bin/duplicacy-backup"
fi

capture_archive="$BUNDLE_ROOT/ui-surface-captures-$STAMP.tar.gz"
tar -czf "$capture_archive" -C "$BUNDLE_ROOT" "captures/$STAMP"

cat <<EOF

Capture complete.
  Summary : $SUMMARY
  Outputs : $CAPTURE_DIR
  Archive : $capture_archive

Review with:
  less '$SUMMARY'
  ls -1 '$CAPTURE_DIR'/*.txt
EOF

if [ "$unexpected_count" -gt 0 ]; then
    echo "Unexpected capture outcomes: $unexpected_count" >&2
    exit 1
fi
