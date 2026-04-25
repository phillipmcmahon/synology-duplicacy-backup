#!/bin/sh

set -eu

RUN_ID=""
KIND="release"
POC_NAME=""
VERSION=""
BUILD_TIME=""
GOOS="linux"
GOARCH="amd64"
GOARM=""
INSTRUCTIONS=""
REPO_ROOT=""

usage() {
    cat <<'EOF'
Usage: ./scripts/package-test-bundle.sh [OPTIONS]

Build one local NAS test package into a structured per-run folder and create a
structured bundle containing the package, checksum, and smoke-test instructions.

Options:
  --run-id <value>       Required folder/bundle id, for example restore-smoke-20260425160000
  --kind <value>         release or poc (default: release)
  --poc-name <value>     Required when --kind poc; creates build/test-packages/poc/<name>/<run-id>
  --version <value>      Version string embedded into the binary
  --build-time <value>   Build timestamp in UTC RFC3339 form
  --goos <value>         Target GOOS (default: linux)
  --goarch <value>       Target GOARCH (default: amd64)
  --goarm <value>        Target GOARM (for GOARCH=arm)
  --instructions <path>  Markdown instructions to include in the bundle
  --repo-root <path>     Repository root (default: script parent directory)
  --help                 Show this help text
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

safe_id() {
    value="$1"
    case "$value" in
        ""|*/*|*..*|.*|*-)
            return 1
            ;;
    esac
    printf '%s\n' "$value" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]*$'
}

archive_without_macos_metadata() {
    archive="$1"
    parent="$2"
    entry="$3"
    rm -f "$archive"
    if COPYFILE_DISABLE=1 tar --disable-copyfile --no-xattrs -czf "$archive" -C "$parent" "$entry" 2>/dev/null; then
        return 0
    fi
    rm -f "$archive"
    if COPYFILE_DISABLE=1 tar --no-xattrs -czf "$archive" -C "$parent" "$entry" 2>/dev/null; then
        return 0
    fi
    rm -f "$archive"
    COPYFILE_DISABLE=1 tar -czf "$archive" -C "$parent" "$entry"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --run-id)
            [ "$#" -ge 2 ] || fail "--run-id requires a value"
            RUN_ID="$2"
            shift 2
            ;;
        --kind)
            [ "$#" -ge 2 ] || fail "--kind requires a value"
            KIND="$2"
            shift 2
            ;;
        --poc-name)
            [ "$#" -ge 2 ] || fail "--poc-name requires a value"
            POC_NAME="$2"
            shift 2
            ;;
        --version)
            [ "$#" -ge 2 ] || fail "--version requires a value"
            VERSION="$2"
            shift 2
            ;;
        --build-time)
            [ "$#" -ge 2 ] || fail "--build-time requires a value"
            BUILD_TIME="$2"
            shift 2
            ;;
        --goos)
            [ "$#" -ge 2 ] || fail "--goos requires a value"
            GOOS="$2"
            shift 2
            ;;
        --goarch)
            [ "$#" -ge 2 ] || fail "--goarch requires a value"
            GOARCH="$2"
            shift 2
            ;;
        --goarm)
            [ "$#" -ge 2 ] || fail "--goarm requires a value"
            GOARM="$2"
            shift 2
            ;;
        --instructions)
            [ "$#" -ge 2 ] || fail "--instructions requires a value"
            INSTRUCTIONS="$2"
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

[ -n "$RUN_ID" ] || fail "--run-id is required"
[ -n "$VERSION" ] || fail "--version is required"
[ -n "$BUILD_TIME" ] || fail "--build-time is required"
[ -n "$INSTRUCTIONS" ] || fail "--instructions is required"
safe_id "$RUN_ID" || fail "invalid --run-id: $RUN_ID"

case "$KIND" in
    release)
        REL_OUTPUT_DIR="build/test-packages/release/$RUN_ID"
        ;;
    poc)
        [ -n "$POC_NAME" ] || fail "--poc-name is required when --kind poc"
        safe_id "$POC_NAME" || fail "invalid --poc-name: $POC_NAME"
        REL_OUTPUT_DIR="build/test-packages/poc/$POC_NAME/$RUN_ID"
        ;;
    *)
        fail "--kind must be release or poc"
        ;;
esac

ROOT="${REPO_ROOT:-$(repo_root_default)}"
[ -f "$ROOT/README.md" ] || fail "repo root not valid: $ROOT"
[ -f "$INSTRUCTIONS" ] || fail "instructions file not found: $INSTRUCTIONS"

OUTPUT_DIR="$ROOT/$REL_OUTPUT_DIR"
[ ! -e "$OUTPUT_DIR" ] || fail "output directory already exists: $OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

docker_output_dir="/work/$REL_OUTPUT_DIR"
package_args="--version $VERSION --build-time $BUILD_TIME --goos $GOOS --goarch $GOARCH --output-dir $docker_output_dir"
if [ -n "$GOARM" ]; then
    package_args="$package_args --goarm $GOARM"
fi

# shellcheck disable=SC2086
package_output="$(cd "$ROOT" && sh ./scripts/package-linux-docker.sh $package_args)"
printf '%s\n' "$package_output"

basename="$(printf '%s\n' "$package_output" | tail -n 1)"
archive="$OUTPUT_DIR/$basename.tar.gz"
checksum="$OUTPUT_DIR/$basename.tar.gz.sha256"
[ -f "$archive" ] || fail "expected package archive missing: $archive"
[ -f "$checksum" ] || fail "expected package checksum missing: $checksum"

instructions_name="$(basename "$INSTRUCTIONS")"
cp "$INSTRUCTIONS" "$OUTPUT_DIR/$instructions_name"

bundle_dir="$OUTPUT_DIR/${RUN_ID}_bundle"
mkdir "$bundle_dir"
mkdir "$bundle_dir/artifacts" "$bundle_dir/checksums" "$bundle_dir/instructions"
cp "$archive" "$bundle_dir/artifacts/"
cp "$checksum" "$bundle_dir/checksums/"
cp "$OUTPUT_DIR/$instructions_name" "$bundle_dir/instructions/"

cat > "$bundle_dir/README.md" <<EOF
# $RUN_ID

Start with:

\`\`\`bash
less instructions/$instructions_name
\`\`\`

Contents:

- \`artifacts/\` contains the Linux package tarball.
- \`checksums/\` contains the package checksum.
- \`instructions/\` contains the NAS smoke-test procedure.
EOF

find "$bundle_dir" "$OUTPUT_DIR" -name '.DS_Store' -o -name '._*' | xargs rm -f 2>/dev/null || true

bundle_archive="$OUTPUT_DIR/${RUN_ID}_bundle.tar.gz"
archive_without_macos_metadata "$bundle_archive" "$OUTPUT_DIR" "${RUN_ID}_bundle"
shasum -a 256 "$bundle_archive" > "$bundle_archive.sha256"

if tar -tzf "$bundle_archive" | grep -E '(^|/)\._|\.DS_Store' >/dev/null; then
    fail "bundle contains macOS metadata files: $bundle_archive"
fi

cat <<EOF
Test package folder: $REL_OUTPUT_DIR
Bundle: $REL_OUTPUT_DIR/${RUN_ID}_bundle.tar.gz
Bundle checksum: $REL_OUTPUT_DIR/${RUN_ID}_bundle.tar.gz.sha256
EOF
