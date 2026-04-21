#!/bin/sh

set -eu

REPO="phillipmcmahon/synology-duplicacy-backup"
HOST="homestorage"
REMOTE_ROOT="/volume1/homes/phillipmcmahon/code/duplicacy-backup"
TAG=""
ISSUE=""
STAGE_DIR=""
VERIFY_ATTESTATIONS=1

usage() {
    cat <<'EOF'
Usage: ./scripts/finalize-release.sh --tag <value> [OPTIONS]

Mirror a published GitHub release to homestorage latest/<tag>, run the full
release verification gate, and print a release-issue closure summary.

Options:
  --tag <value>         Release tag to finalize (for example v4.4.0)
  --repo <value>        GitHub repository in owner/name form
                        (default: phillipmcmahon/synology-duplicacy-backup)
  --host <value>        SSH host used for mirroring and verification
                        (default: homestorage)
  --remote-root <path>  Remote base directory
                        (default: /volume1/homes/phillipmcmahon/code/duplicacy-backup)
  --stage-dir <path>    Reuse an existing local staging directory for mirroring
  --issue <number>      Release issue number to include in the closure summary
  --skip-attestations   Skip GitHub release and asset attestation checks
                        (only for historical releases)
  --help                Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
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
        --stage-dir)
            [ "$#" -ge 2 ] || fail "--stage-dir requires a value"
            STAGE_DIR="$2"
            shift 2
            ;;
        --issue)
            [ "$#" -ge 2 ] || fail "--issue requires a value"
            ISSUE="$2"
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

SCRIPT_DIR="$(script_dir)"
MIRROR_SCRIPT="${MIRROR_RELEASE_SCRIPT:-$SCRIPT_DIR/mirror-release-assets.sh}"
VERIFY_SCRIPT="${VERIFY_RELEASE_SCRIPT:-$SCRIPT_DIR/verify-release.sh}"
REMOTE_DIR="$REMOTE_ROOT/latest/$TAG"

[ -f "$MIRROR_SCRIPT" ] || fail "mirror script not found: $MIRROR_SCRIPT"
[ -f "$VERIFY_SCRIPT" ] || fail "verify script not found: $VERIFY_SCRIPT"

if [ -n "$STAGE_DIR" ]; then
    sh "$MIRROR_SCRIPT" \
        --tag "$TAG" \
        --repo "$REPO" \
        --host "$HOST" \
        --remote-root "$REMOTE_ROOT" \
        --stage-dir "$STAGE_DIR"
else
    sh "$MIRROR_SCRIPT" \
        --tag "$TAG" \
        --repo "$REPO" \
        --host "$HOST" \
        --remote-root "$REMOTE_ROOT"
fi

if [ "$VERIFY_ATTESTATIONS" -eq 1 ]; then
    sh "$VERIFY_SCRIPT" \
        --tag "$TAG" \
        --repo "$REPO" \
        --host "$HOST" \
        --remote-root "$REMOTE_ROOT"
    ATTESTATION_RESULT="verified"
    VERIFY_COMMAND="scripts/verify-release.sh --tag $TAG"
else
    sh "$VERIFY_SCRIPT" \
        --tag "$TAG" \
        --repo "$REPO" \
        --host "$HOST" \
        --remote-root "$REMOTE_ROOT" \
        --skip-attestations
    ATTESTATION_RESULT="skipped"
    VERIFY_COMMAND="scripts/verify-release.sh --tag $TAG --skip-attestations"
fi

RELEASE_URL="$(gh release view "$TAG" --repo "$REPO" --json url --jq .url)"

echo
echo "Release closure summary"
echo "  Tag                  : $TAG"
echo "  Release URL          : $RELEASE_URL"
echo "  Remote mirror        : $HOST:$REMOTE_DIR"
echo "  Full verification    : passed ($VERIFY_COMMAND)"
echo "  Release attestations : $ATTESTATION_RESULT"
if [ -n "$ISSUE" ]; then
    echo "  Release issue        : #$ISSUE"
fi
echo
echo "Issue comment:"
echo "$TAG release closure complete."
echo
echo "- GitHub release: $RELEASE_URL"
echo "- NAS mirror: $HOST:$REMOTE_DIR"
echo "- Full release verification: passed with \`$VERIFY_COMMAND\`"
echo "- Release attestations: $ATTESTATION_RESULT"
