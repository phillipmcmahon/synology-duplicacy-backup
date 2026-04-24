#!/bin/sh

set -eu

REPO_ROOT=""
OUTPUT_ROOT=""
FIXTURE=""
GUIDE=""
STAMP=""
VERSION=""

usage() {
    cat <<'EOF'
Usage: ./scripts/package-restore-picker-poc.sh --fixture <path> --guide <path> [OPTIONS]

Build a clean linux/amd64 restore-picker-poc test bundle under the structured
POC test-packages tree.

Options:
  --fixture <path>      Fixture text file to include in the bundle
  --guide <path>        Review guide markdown file to include in the bundle
  --output-root <path>  Output root directory (default: build/test-packages/poc/restore-picker)
  --stamp <value>       Override UTC timestamp suffix (default: now)
  --version <value>     Override version string (default: git describe)
  --repo-root <path>    Repository root (default: script parent directory)
  --help                Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

script_dir() {
    CDPATH= cd -- "$(dirname -- "$0")" && pwd
}

repo_root_default() {
    dirname "$(script_dir)"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --fixture)
            [ "$#" -ge 2 ] || fail "--fixture requires a value"
            FIXTURE="$2"
            shift 2
            ;;
        --guide)
            [ "$#" -ge 2 ] || fail "--guide requires a value"
            GUIDE="$2"
            shift 2
            ;;
        --output-root)
            [ "$#" -ge 2 ] || fail "--output-root requires a value"
            OUTPUT_ROOT="$2"
            shift 2
            ;;
        --stamp)
            [ "$#" -ge 2 ] || fail "--stamp requires a value"
            STAMP="$2"
            shift 2
            ;;
        --version)
            [ "$#" -ge 2 ] || fail "--version requires a value"
            VERSION="$2"
            shift 2
            ;;
        --repo-root)
            [ "$#" -ge 2 ] || fail "--repo-root requires a value"
            REPO_ROOT="$2"
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

[ -n "$FIXTURE" ] || fail "--fixture is required"
[ -n "$GUIDE" ] || fail "--guide is required"

REPO_ROOT="${REPO_ROOT:-$(repo_root_default)}"
OUTPUT_ROOT="${OUTPUT_ROOT:-$REPO_ROOT/build/test-packages/poc/restore-picker}"
STAMP="${STAMP:-$(date -u '+%Y%m%d%H%M%S')}"
VERSION="${VERSION:-$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"

[ -f "$FIXTURE" ] || fail "fixture file not found: $FIXTURE"
[ -f "$GUIDE" ] || fail "guide file not found: $GUIDE"
[ -f "$REPO_ROOT/go.mod" ] || fail "repo root not valid: $REPO_ROOT"

BUNDLE_NAME="restore-picker-poc_${VERSION}_${STAMP}"
BINARY_NAME="${BUNDLE_NAME}_linux_amd64"
BUNDLE_DIR="$OUTPUT_ROOT/$BUNDLE_NAME"
ARCHIVE_PATH="$OUTPUT_ROOT/${BUNDLE_NAME}_bundle.tar.gz"
BINARY_PATH="$BUNDLE_DIR/$BINARY_NAME"
FIXTURE_TARGET="$BUNDLE_DIR/$(basename "$FIXTURE")"
GUIDE_TARGET="$BUNDLE_DIR/$(basename "$GUIDE")"
MANIFEST_PATH="$BUNDLE_DIR/SHA256SUMS"

mkdir -p "$OUTPUT_ROOT"
[ ! -e "$BUNDLE_DIR" ] || fail "bundle directory already exists: $BUNDLE_DIR"
[ ! -e "$ARCHIVE_PATH" ] || fail "bundle archive already exists: $ARCHIVE_PATH"

mkdir -p "$BUNDLE_DIR"

(
    cd "$REPO_ROOT"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BINARY_PATH" ./cmd/restore-picker-poc
)
cp "$FIXTURE" "$FIXTURE_TARGET"
cp "$GUIDE" "$GUIDE_TARGET"

(
    cd "$BUNDLE_DIR"
    shasum -a 256 "$(basename "$BINARY_PATH")" "$(basename "$FIXTURE_TARGET")" "$(basename "$GUIDE_TARGET")" > "$(basename "$MANIFEST_PATH")"
)

COPYFILE_DISABLE=1 COPY_EXTENDED_ATTRIBUTES_DISABLE=1 tar \
    --exclude='.DS_Store' \
    --exclude='._*' \
    -czf "$ARCHIVE_PATH" \
    -C "$OUTPUT_ROOT" \
    "$(basename "$BUNDLE_DIR")"

printf '%s\n' "$BUNDLE_DIR"
printf '%s\n' "$ARCHIVE_PATH"
