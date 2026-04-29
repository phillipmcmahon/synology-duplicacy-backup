#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
RUN_ID="${RUN_ID:-ci-ui-surface-smoke}"
VERSION="${VERSION:-ci-ui-surface-smoke}"
BUILD_TIME="${BUILD_TIME:-2026-01-01T00:00:00Z}"
OUTPUT_DIR="$ROOT/build/test-packages/release/$RUN_ID"
BUNDLE_DIR="$OUTPUT_DIR/${RUN_ID}_bundle"
BUNDLE="$OUTPUT_DIR/${RUN_ID}_bundle.tar.gz"
CHECKSUM="$BUNDLE.sha256"

fail() {
    echo "Error: $*" >&2
    exit 1
}

assert_file() {
    [ -f "$1" ] || fail "expected file missing: $1"
}

assert_executable() {
    [ -x "$1" ] || fail "expected executable missing: $1"
}

rm -rf "$OUTPUT_DIR"

sh "$ROOT/scripts/test-ui-surface-smoke.sh"
sh "$ROOT/scripts/package-ui-surface-smoke.sh" \
    --run-id "$RUN_ID" \
    --version "$VERSION" \
    --build-time "$BUILD_TIME" \
    --goos linux \
    --goarch amd64

assert_file "$BUNDLE"
assert_file "$CHECKSUM"
assert_file "$BUNDLE_DIR/setup-env.sh"
assert_file "$BUNDLE_DIR/instructions/smoke-test.md"
assert_executable "$BUNDLE_DIR/run-ui-surface-smoke.sh"

sh -n "$BUNDLE_DIR/setup-env.sh"
sh -n "$BUNDLE_DIR/run-ui-surface-smoke.sh"
grep -F 'CAPTURE_COLOUR="${CAPTURE_COLOUR:-1}"' "$BUNDLE_DIR/run-ui-surface-smoke.sh" >/dev/null ||
    fail "UI smoke runner does not default colour capture on"

tar -tzf "$BUNDLE" | grep -F "${RUN_ID}_bundle/run-ui-surface-smoke.sh" >/dev/null ||
    fail "bundle archive does not contain run-ui-surface-smoke.sh"
tar -tzf "$BUNDLE" | grep -F "${RUN_ID}_bundle/setup-env.sh" >/dev/null ||
    fail "bundle archive does not contain setup-env.sh"
if tar -tzf "$BUNDLE" | grep -E '(^|/)\._|\.DS_Store' >/dev/null; then
    fail "bundle archive contains macOS metadata"
fi

checksum_file="$(mktemp)"
trap 'rm -f "$checksum_file"' EXIT INT TERM
sed "s#  $BUNDLE#  $(basename "$BUNDLE")#" "$CHECKSUM" >"$checksum_file"
(cd "$OUTPUT_DIR" && shasum -a 256 -c "$checksum_file")

echo "ui-surface CI smoke passed"
