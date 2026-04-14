#!/bin/sh

set -eu

REPO="phillipmcmahon/synology-duplicacy-backup"
HOST="homestorage"
REMOTE_ROOT="/volume1/homes/phillipmcmahon/code/duplicacy-backup"
TAG=""
STAGE_DIR=""

usage() {
    cat <<'EOF'
Usage: ./scripts/mirror-release-assets.sh --tag <value> [OPTIONS]

Download the published GitHub release assets and source archives for one tag,
then mirror the full artefact set to the homestorage release directory.
The mirrored set includes GitHub release assets plus `Source code (zip)` and
`Source code (tar.gz)`.

Options:
  --tag <value>         Release tag to mirror (for example v4.1.4)
  --repo <value>        GitHub repository in owner/name form
                        (default: phillipmcmahon/synology-duplicacy-backup)
  --host <value>        SSH host used for mirroring (default: homestorage)
  --remote-root <path>  Remote base directory
                        (default: /volume1/homes/phillipmcmahon/code/duplicacy-backup)
  --stage-dir <path>    Reuse an existing local staging directory
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

cleanup() {
    if [ -n "${TEMP_STAGE_DIR:-}" ] && [ -d "$TEMP_STAGE_DIR" ]; then
        rm -rf "$TEMP_STAGE_DIR"
    fi
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

require_command gh
require_command ssh
require_command tar
require_command mktemp
require_command find
require_command basename

if [ -z "$STAGE_DIR" ]; then
    TEMP_STAGE_DIR="$(mktemp -d)"
    STAGE_DIR="$TEMP_STAGE_DIR"
fi

[ -d "$STAGE_DIR" ] || fail "stage directory does not exist: $STAGE_DIR"

trap cleanup EXIT INT TERM

gh release download "$TAG" --repo "$REPO" --dir "$STAGE_DIR"
gh release download "$TAG" --repo "$REPO" --archive zip --output "$STAGE_DIR/Source code (zip)"
gh release download "$TAG" --repo "$REPO" --archive tar.gz --output "$STAGE_DIR/Source code (tar.gz)"

REMOTE_DIR="$REMOTE_ROOT/$TAG"

ssh "$HOST" "mkdir -p '$REMOTE_DIR'"

(
    cd "$STAGE_DIR"
    export COPYFILE_DISABLE=1
    export COPY_EXTENDED_ATTRIBUTES_DISABLE=1
    tar --format ustar -cf - .
) | ssh "$HOST" "tar -xf - -C '$REMOTE_DIR'"

echo "Mirrored $TAG to $HOST:$REMOTE_DIR"
echo "Artefacts:"
find "$STAGE_DIR" -maxdepth 1 -type f -exec basename {} \; | sort
