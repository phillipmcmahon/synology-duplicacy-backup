#!/bin/sh

set -eu

RUN_ID=""
VERSION=""
BUILD_TIME=""
GOOS="linux"
GOARCH="amd64"
GOARM=""
REPO_ROOT=""
DEFAULT_LABEL="homes"
DEFAULT_TARGET="onsite-garage"
DEFAULT_WORKSPACE_ROOT="/volume1/restore-drills"

usage() {
    cat <<'EOF'
Usage: ./scripts/package-ui-surface-smoke.sh [OPTIONS]

Build a structured NAS UI surface smoke bundle for release-candidate output
review. The bundle includes the Linux package, setup-env.sh, instructions, and
run-ui-surface-smoke.sh.

Options:
  --run-id <value>       Bundle id. Defaults to ui-surface-smoke-<UTC timestamp>
  --version <value>      Version string embedded into the binary
  --build-time <value>   Build timestamp in UTC RFC3339 form
  --goos <value>         Target GOOS (default: linux)
  --goarch <value>       Target GOARCH (default: amd64)
  --goarm <value>        Target GOARM (for GOARCH=arm)
  --default-label <name> LABEL default written to setup-env.sh
  --default-target <name>
                         TARGET default written to setup-env.sh
  --default-workspace-root <path>
                         WORKSPACE_ROOT default written to setup-env.sh
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
        --default-label)
            [ "$#" -ge 2 ] || fail "--default-label requires a value"
            DEFAULT_LABEL="$2"
            shift 2
            ;;
        --default-target)
            [ "$#" -ge 2 ] || fail "--default-target requires a value"
            DEFAULT_TARGET="$2"
            shift 2
            ;;
        --default-workspace-root)
            [ "$#" -ge 2 ] || fail "--default-workspace-root requires a value"
            DEFAULT_WORKSPACE_ROOT="$2"
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

ROOT="${REPO_ROOT:-$(repo_root_default)}"
[ -f "$ROOT/README.md" ] || fail "repo root not valid: $ROOT"

if [ -z "$RUN_ID" ]; then
    RUN_ID="ui-surface-smoke-$(date -u '+%Y%m%d%H%M%S')"
fi
if [ -z "$BUILD_TIME" ]; then
    BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
fi
if [ -z "$VERSION" ]; then
    describe="$(cd "$ROOT" && git describe --tags --always --dirty 2>/dev/null || printf 'dev')"
    VERSION="$describe-$RUN_ID"
fi

INSTRUCTIONS="$ROOT/docs/ui-surface-smoke.md"
[ -f "$INSTRUCTIONS" ] || fail "instructions file not found: $INSTRUCTIONS"

package_args="
  --run-id $RUN_ID
  --version $VERSION
  --build-time $BUILD_TIME
  --goos $GOOS
  --goarch $GOARCH
  --instructions $INSTRUCTIONS
  --default-label $DEFAULT_LABEL
  --default-target $DEFAULT_TARGET
  --default-workspace-root $DEFAULT_WORKSPACE_ROOT
  --repo-root $ROOT
"
if [ -n "$GOARM" ]; then
    package_args="$package_args --goarm $GOARM"
fi

# shellcheck disable=SC2086
package_output="$(cd "$ROOT" && sh ./scripts/package-test-bundle.sh $package_args)"
printf '%s\n' "$package_output"

bundle_dir="$ROOT/build/test-packages/release/$RUN_ID/${RUN_ID}_bundle"
[ -d "$bundle_dir" ] || fail "bundle directory not found: $bundle_dir"

cp "$ROOT/scripts/ui-surface-smoke-runner.sh" "$bundle_dir/run-ui-surface-smoke.sh"
chmod 755 "$bundle_dir/run-ui-surface-smoke.sh"

output_dir="$ROOT/build/test-packages/release/$RUN_ID"
bundle_archive="$output_dir/${RUN_ID}_bundle.tar.gz"
archive_without_macos_metadata "$bundle_archive" "$output_dir" "${RUN_ID}_bundle"
shasum -a 256 "$bundle_archive" > "$bundle_archive.sha256"

if tar -tzf "$bundle_archive" | grep -E '(^|/)\._|\.DS_Store' >/dev/null; then
    fail "bundle contains macOS metadata files: $bundle_archive"
fi

cat <<EOF
UI surface smoke bundle ready:
  Folder         : build/test-packages/release/$RUN_ID
  Bundle         : build/test-packages/release/$RUN_ID/${RUN_ID}_bundle.tar.gz
  Bundle checksum: build/test-packages/release/$RUN_ID/${RUN_ID}_bundle.tar.gz.sha256
EOF
