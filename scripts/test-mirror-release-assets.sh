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

assert_file() {
    path="$1"
    [ -f "$path" ] || fail "expected file missing: $path"
}

assert_dir() {
    path="$1"
    [ -d "$path" ] || fail "expected directory missing: $path"
}

assert_not_dir() {
    path="$1"
    [ ! -d "$path" ] || fail "unexpected directory exists: $path"
}

mkdir -p "$TMP_DIR/bin" "$TMP_DIR/remote/v9.8.0" "$TMP_DIR/remote/latest/v9.9.8"
mkdir -p "$TMP_DIR/remote/archive"

cat >"$TMP_DIR/bin/gh" <<'EOF'
#!/bin/sh
set -eu

[ "$1" = "release" ] || {
    echo "unexpected gh invocation: $*" >&2
    exit 1
}
[ "$2" = "download" ] || {
    echo "unexpected gh release invocation: $*" >&2
    exit 1
}

dir=""
output=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        --dir)
            dir="$2"
            shift 2
            ;;
        --output)
            output="$2"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

if [ -n "$output" ]; then
    mkdir -p "$(dirname "$output")"
    printf 'archive\n' >"$output"
    exit 0
fi

[ -n "$dir" ] || {
    echo "missing --dir" >&2
    exit 1
}

cat >"$dir/SHA256SUMS.txt" <<'ASSETS'
checksums
ASSETS
cat >"$dir/duplicacy-backup_9.9.9_linux_amd64.tar.gz" <<'ASSETS'
amd64
ASSETS
EOF

cat >"$TMP_DIR/bin/ssh" <<'EOF'
#!/bin/sh
set -eu
shift
exec sh -c "$*"
EOF

chmod +x "$TMP_DIR/bin/gh" "$TMP_DIR/bin/ssh"

PATH="$TMP_DIR/bin:$PATH" \
    sh "$ROOT/scripts/mirror-release-assets.sh" \
        --tag v9.9.9 \
        --repo example/repo \
        --host fake-host \
        --remote-root "$TMP_DIR/remote" >"$TMP_DIR/output.txt"

assert_dir "$TMP_DIR/remote/latest/v9.9.9"
assert_dir "$TMP_DIR/remote/archive/v9.8.0"
assert_dir "$TMP_DIR/remote/archive/v9.9.8"
assert_not_dir "$TMP_DIR/remote/v9.8.0"
assert_not_dir "$TMP_DIR/remote/latest/v9.9.8"
assert_file "$TMP_DIR/remote/latest/v9.9.9/SHA256SUMS.txt"
assert_file "$TMP_DIR/remote/latest/v9.9.9/Source code (zip)"
assert_file "$TMP_DIR/remote/latest/v9.9.9/Source code (tar.gz)"
assert_file "$TMP_DIR/remote/latest/v9.9.9/duplicacy-backup_9.9.9_linux_amd64.tar.gz"

grep -F "Mirrored v9.9.9 to fake-host:$TMP_DIR/remote/latest/v9.9.9" "$TMP_DIR/output.txt" >/dev/null || {
    echo "mirror output did not include latest path" >&2
    sed -n '1,120p' "$TMP_DIR/output.txt" >&2
    exit 1
}

echo "mirror-release-assets script tests passed"
