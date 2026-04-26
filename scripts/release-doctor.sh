#!/bin/sh

set -eu

REPO="phillipmcmahon/synology-duplicacy-backup"
TAG=""
ISSUE=""
HOST="homestorage"
REMOTE_ROOT="/volume1/homes/phillipmcmahon/code/duplicacy-backup"
VERIFY_ATTESTATIONS=1

usage() {
    cat <<'EOF'
Usage: ./scripts/release-doctor.sh --tag <value> --issue <number> [OPTIONS]

Run the objective release-complete gate after a release has been finalized.

Checks:
  - local tree is clean and on main
  - local main matches origin/main
  - local and remote tags exist and point at the same commit
  - GitHub release and NAS mirror pass scripts/verify-release.sh
  - release issue contains closure evidence
  - project board audit passes

Options:
  --tag <value>         Release tag to check (for example v9.0.0)
  --issue <number>      Release issue number that should contain closure evidence
  --repo <owner/name>   GitHub repository
  --host <value>        SSH host used for mirror verification
  --remote-root <path>  Remote release mirror root
  --skip-attestations   Skip attestation checks for historical releases only
  --help                Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

script_dir() {
    CDPATH= cd "$(dirname "$0")" && pwd
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --tag)
            [ "$#" -ge 2 ] || fail "--tag requires a value"
            TAG="$2"
            shift 2
            ;;
        --issue)
            [ "$#" -ge 2 ] || fail "--issue requires a value"
            ISSUE="$2"
            shift 2
            ;;
        --repo)
            [ "$#" -ge 2 ] || fail "--repo requires a value"
            REPO="$2"
            shift 2
            ;;
        --host)
            [ "$#" -ge 2 ] || fail "--host requires a value"
            HOST="$2"
            shift 2
            ;;
        --remote-root)
            [ "$#" -ge 2 ] || fail "--remote-root requires a value"
            REMOTE_ROOT="$2"
            shift 2
            ;;
        --skip-attestations)
            VERIFY_ATTESTATIONS=0
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

[ -n "$TAG" ] || fail "--tag is required"
[ -n "$ISSUE" ] || fail "--issue is required"

require_command gh
require_command git
require_command jq

ROOT="$(git rev-parse --show-toplevel)"
SCRIPT_DIR="$(script_dir)"
VERIFY_SCRIPT="${VERIFY_RELEASE_SCRIPT:-$SCRIPT_DIR/verify-release.sh}"
BOARD_AUDIT_SCRIPT="${PROJECT_BOARD_AUDIT_SCRIPT:-$SCRIPT_DIR/project-board-audit.sh}"

[ -f "$VERIFY_SCRIPT" ] || fail "verify script not found: $VERIFY_SCRIPT"
[ -f "$BOARD_AUDIT_SCRIPT" ] || fail "project board audit script not found: $BOARD_AUDIT_SCRIPT"

[ -z "$(git -C "$ROOT" status --short)" ] || fail "release doctor requires a clean git tree"
[ "$(git -C "$ROOT" branch --show-current)" = "main" ] || fail "release doctor must run from main"

git -C "$ROOT" fetch --quiet origin main --tags

local_head="$(git -C "$ROOT" rev-parse HEAD)"
origin_head="$(git -C "$ROOT" rev-parse origin/main)"
[ "$local_head" = "$origin_head" ] || fail "local main does not match origin/main"

local_tag="$(git -C "$ROOT" rev-list -n 1 "$TAG")"
remote_tag="$(git -C "$ROOT" ls-remote "https://github.com/$REPO.git" "refs/tags/$TAG^{}" | awk '{print $1}')"
[ -n "$remote_tag" ] || fail "remote tag not found: $TAG"
[ "$local_tag" = "$remote_tag" ] || fail "local and remote tag commits differ for $TAG"

if [ "$VERIFY_ATTESTATIONS" -eq 1 ]; then
    sh "$VERIFY_SCRIPT" --tag "$TAG" --repo "$REPO" --host "$HOST" --remote-root "$REMOTE_ROOT"
else
    sh "$VERIFY_SCRIPT" --tag "$TAG" --repo "$REPO" --host "$HOST" --remote-root "$REMOTE_ROOT" --skip-attestations
fi

issue_json="$(mktemp)"
cleanup() {
    rm -f "$issue_json"
}
trap cleanup EXIT INT TERM

gh issue view "$ISSUE" --repo "$REPO" --json number,state,title,comments >"$issue_json"

jq -e --arg tag "$TAG" '
  .comments[].body
  | contains($tag + " release closure complete.")
    and contains("NAS mirror:")
    and contains("Full release verification:")
' "$issue_json" >/dev/null || fail "release issue #$ISSUE does not contain closure evidence for $TAG"

sh "$BOARD_AUDIT_SCRIPT"

echo "Release doctor passed for $TAG"
