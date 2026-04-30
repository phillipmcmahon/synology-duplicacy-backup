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

mkdir -p "$TMP_DIR/bin" "$TMP_DIR/mirror/latest/v10.0.3" "$TMP_DIR/mirror/latest/v10.0.4"

cat >"$TMP_DIR/bin/gh" <<'EOF'
#!/bin/sh
set -eu

if [ "$1" = "release" ] && [ "$2" = "view" ]; then
    tag="$3"
    case "$tag" in
        v10.0.3)
            body='## Highlights\n\n## Validation\n\n## Coverage\n'
            ;;
        v10.0.4)
            body='## Highlights\n\n## Validation\n\n## Coverage\n'
            ;;
        *)
            echo "unexpected tag: $tag" >&2
            exit 1
            ;;
    esac
    jq -n \
        --arg tag "$tag" \
        --arg body "$body" \
        '{
          tagName: $tag,
          name: $tag,
          body: $body,
          isDraft: false,
          isPrerelease: false,
          url: ("https://example.invalid/" + $tag),
          assets: [
            {name:"SHA256SUMS.txt"},
            {name:("duplicacy-backup_" + ($tag | ltrimstr("v")) + "_linux_amd64.tar.gz")},
            {name:("duplicacy-backup_" + ($tag | ltrimstr("v")) + "_linux_amd64.tar.gz.sha256")},
            {name:("duplicacy-backup_" + ($tag | ltrimstr("v")) + "_linux_arm64.tar.gz")},
            {name:("duplicacy-backup_" + ($tag | ltrimstr("v")) + "_linux_arm64.tar.gz.sha256")},
            {name:("duplicacy-backup_" + ($tag | ltrimstr("v")) + "_linux_armv7.tar.gz")},
            {name:("duplicacy-backup_" + ($tag | ltrimstr("v")) + "_linux_armv7.tar.gz.sha256")}
          ]
        }'
    exit 0
fi

echo "unexpected gh invocation: $*" >&2
exit 1
EOF

cat >"$TMP_DIR/bin/git" <<'EOF'
#!/bin/sh
set -eu

if [ "$1" = "rev-parse" ]; then
    echo "/tmp/fake-repo"
    exit 0
fi
if [ "$1" = "-C" ] && [ "$3" = "rev-list" ]; then
    echo "abc123"
    exit 0
fi
if [ "$1" = "ls-remote" ]; then
    tag="${3#refs/tags/}"
    tag="${tag%^{} }"
    echo "abc123 refs/tags/$tag^{}"
    exit 0
fi

echo "unexpected git invocation: $*" >&2
exit 1
EOF

cat >"$TMP_DIR/bin/ssh" <<'EOF'
#!/bin/sh
set -eu
shift
exec sh -c "$*"
EOF

chmod +x "$TMP_DIR/bin/gh" "$TMP_DIR/bin/git" "$TMP_DIR/bin/ssh"

populate_mirror() {
    tag="$1"
    version="${tag#v}"
    dir="$TMP_DIR/mirror/latest/$tag"
    cat >"$dir/SHA256SUMS.txt" <<EOF
checksums
EOF
    cat >"$dir/Source code (tar.gz)" <<EOF
source
EOF
    cat >"$dir/Source code (zip)" <<EOF
source
EOF
    for arch in linux_amd64 linux_arm64 linux_armv7; do
        cat >"$dir/duplicacy-backup_${version}_${arch}.tar.gz" <<EOF
asset
EOF
        cat >"$dir/duplicacy-backup_${version}_${arch}.tar.gz.sha256" <<EOF
checksum
EOF
    done
}

populate_mirror v10.0.3
populate_mirror v10.0.4

PATH="$TMP_DIR/bin:$PATH" \
    sh "$ROOT/scripts/verify-release.sh" \
        --tag v10.0.3 \
        --repo example/repo \
        --host fake-host \
        --remote-root "$TMP_DIR/mirror" \
        --skip-attestations >"$TMP_DIR/v10.0.3.out"

assert_contains "$TMP_DIR/v10.0.3.out" "Verified v10.0.3"

if PATH="$TMP_DIR/bin:$PATH" \
    sh "$ROOT/scripts/verify-release.sh" \
        --tag v10.0.4 \
        --repo example/repo \
        --host fake-host \
        --remote-root "$TMP_DIR/mirror" \
        --skip-attestations >"$TMP_DIR/v10.0.4.out" 2>&1; then
    fail "verify-release should require Operator impact from v10.0.4 onward"
fi

assert_contains "$TMP_DIR/v10.0.4.out" "release notes missing Operator impact section"

echo "verify-release script tests passed"
