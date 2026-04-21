#!/bin/sh

set -eu

ROOT="$(CDPATH= cd "$(dirname "$0")/.." && pwd)"
TMP_DIR="$(mktemp -d)"

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

fail() {
    echo "Error: $*" >&2
    exit 1
}

assert_contains() {
    file="$1"
    pattern="$2"
    grep -F -- "$pattern" "$file" >/dev/null || {
        echo "Expected to find: $pattern" >&2
        echo "Actual output:" >&2
        sed -n '1,160p' "$file" >&2
        exit 1
    }
}

mkdir -p "$TMP_DIR/bin" "$TMP_DIR/stage"

cat >"$TMP_DIR/mirror.sh" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$@" >"$MIRROR_ARGS_OUT"
echo "fake mirror complete"
EOF

cat >"$TMP_DIR/verify.sh" <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$@" >"$VERIFY_ARGS_OUT"
echo "fake verify complete"
EOF

cat >"$TMP_DIR/bin/gh" <<'EOF'
#!/bin/sh
set -eu
if [ "$1" = "release" ] && [ "$2" = "view" ]; then
    echo "https://github.com/example/repo/releases/tag/v9.9.9"
    exit 0
fi
echo "unexpected gh invocation: $*" >&2
exit 1
EOF

chmod +x "$TMP_DIR/mirror.sh" "$TMP_DIR/verify.sh" "$TMP_DIR/bin/gh"

MIRROR_ARGS_OUT="$TMP_DIR/mirror-args.txt"
VERIFY_ARGS_OUT="$TMP_DIR/verify-args.txt"
export MIRROR_ARGS_OUT VERIFY_ARGS_OUT

PATH="$TMP_DIR/bin:$PATH" \
MIRROR_RELEASE_SCRIPT="$TMP_DIR/mirror.sh" \
VERIFY_RELEASE_SCRIPT="$TMP_DIR/verify.sh" \
    sh "$ROOT/scripts/finalize-release.sh" \
        --tag v9.9.9 \
        --repo example/repo \
        --host nas \
        --remote-root /releases \
        --stage-dir "$TMP_DIR/stage" \
        --issue 42 >"$TMP_DIR/output.txt"

assert_contains "$MIRROR_ARGS_OUT" "--stage-dir"
assert_contains "$MIRROR_ARGS_OUT" "$TMP_DIR/stage"
assert_contains "$VERIFY_ARGS_OUT" "--tag"
assert_contains "$VERIFY_ARGS_OUT" "v9.9.9"
assert_contains "$TMP_DIR/output.txt" "Release closure summary"
assert_contains "$TMP_DIR/output.txt" "Remote mirror        : nas:/releases/latest/v9.9.9"
assert_contains "$TMP_DIR/output.txt" "Release attestations : verified"
assert_contains "$TMP_DIR/output.txt" "Release issue        : #42"
assert_contains "$TMP_DIR/output.txt" "Issue comment:"

PATH="$TMP_DIR/bin:$PATH" \
MIRROR_RELEASE_SCRIPT="$TMP_DIR/mirror.sh" \
VERIFY_RELEASE_SCRIPT="$TMP_DIR/verify.sh" \
    sh "$ROOT/scripts/finalize-release.sh" \
        --tag v9.9.9 \
        --repo example/repo \
        --host nas \
        --remote-root /releases \
        --skip-attestations >"$TMP_DIR/output-skip.txt"

assert_contains "$VERIFY_ARGS_OUT" "--skip-attestations"
assert_contains "$TMP_DIR/output-skip.txt" "Release attestations : skipped"

if sh "$ROOT/scripts/finalize-release.sh" >"$TMP_DIR/missing-tag.txt" 2>&1; then
    fail "finalize-release should fail when --tag is missing"
fi
assert_contains "$TMP_DIR/missing-tag.txt" "Error: --tag is required"

echo "finalize-release script tests passed"
