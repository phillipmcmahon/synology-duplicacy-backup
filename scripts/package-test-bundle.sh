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
DEFAULT_CONFIG_DIR=""
DEFAULT_SECRETS_DIR=""
DEFAULT_LABEL="homes"
DEFAULT_TARGET="onsite-garage"
DEFAULT_WORKSPACE_ROOT="/volume1/restore-drills"

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
  --default-config-dir <path>
                         CFG default written to setup-env.sh
  --default-secrets-dir <path>
                         SEC default written to setup-env.sh
  --default-label <name> LABEL default written to setup-env.sh (default: homes)
  --default-target <name>
                         TARGET default written to setup-env.sh (default: onsite-garage)
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

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

shell_quote() {
    printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
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
        --default-config-dir)
            [ "$#" -ge 2 ] || fail "--default-config-dir requires a value"
            DEFAULT_CONFIG_DIR="$2"
            shift 2
            ;;
        --default-secrets-dir)
            [ "$#" -ge 2 ] || fail "--default-secrets-dir requires a value"
            DEFAULT_SECRETS_DIR="$2"
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

case "$GOARCH" in
    arm)
        if [ -n "$GOARM" ]; then
            artifact_platform="${GOOS}-${GOARCH}v${GOARM}"
        else
            artifact_platform="${GOOS}-${GOARCH}"
        fi
        ;;
    *)
        artifact_platform="${GOOS}-${GOARCH}"
        ;;
esac

instructions_source_name="$(basename "$INSTRUCTIONS")"
instructions_name="smoke-test.md"
cp "$INSTRUCTIONS" "$OUTPUT_DIR/$instructions_name"

bundle_dir="$OUTPUT_DIR/${RUN_ID}_bundle"
mkdir "$bundle_dir"
mkdir "$bundle_dir/artifacts" "$bundle_dir/checksums" "$bundle_dir/instructions" "$bundle_dir/metadata"
mkdir "$bundle_dir/artifacts/$artifact_platform" "$bundle_dir/checksums/$artifact_platform"
cp "$archive" "$bundle_dir/artifacts/$artifact_platform/"
cp "$checksum" "$bundle_dir/checksums/$artifact_platform/"
cp "$OUTPUT_DIR/$instructions_name" "$bundle_dir/instructions/"

commit="$(cd "$ROOT" && git rev-parse HEAD 2>/dev/null || printf 'unknown')"
describe="$(cd "$ROOT" && git describe --tags --always --dirty 2>/dev/null || printf 'unknown')"
created_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

cat > "$bundle_dir/metadata/build.json" <<EOF
{
  "run_id": "$(json_escape "$RUN_ID")",
  "kind": "$(json_escape "$KIND")",
  "poc_name": "$(json_escape "$POC_NAME")",
  "version": "$(json_escape "$VERSION")",
  "build_time": "$(json_escape "$BUILD_TIME")",
  "created_at": "$(json_escape "$created_at")",
  "git_commit": "$(json_escape "$commit")",
  "git_describe": "$(json_escape "$describe")",
  "goos": "$(json_escape "$GOOS")",
  "goarch": "$(json_escape "$GOARCH")",
  "goarm": "$(json_escape "$GOARM")",
  "artifact_platform": "$(json_escape "$artifact_platform")",
  "package_basename": "$(json_escape "$basename")",
  "instructions": "instructions/$(json_escape "$instructions_name")",
  "instructions_source": "$(json_escape "$instructions_source_name")"
}
EOF

quoted_default_config_dir="$(shell_quote "$DEFAULT_CONFIG_DIR")"
quoted_default_secrets_dir="$(shell_quote "$DEFAULT_SECRETS_DIR")"
quoted_default_label="$(shell_quote "$DEFAULT_LABEL")"
quoted_default_target="$(shell_quote "$DEFAULT_TARGET")"
quoted_default_workspace_root="$(shell_quote "$DEFAULT_WORKSPACE_ROOT")"

cat > "$bundle_dir/setup-env.sh" <<EOF
# Source this file from the extracted bundle root:
#   . ./setup-env.sh
#
# Existing CFG, SEC, LABEL, TARGET, and WORKSPACE_ROOT values are preserved.

DEFAULT_CFG=$quoted_default_config_dir
DEFAULT_SEC=$quoted_default_secrets_dir
DEFAULT_LABEL=$quoted_default_label
DEFAULT_TARGET=$quoted_default_target
DEFAULT_WORKSPACE_ROOT=$quoted_default_workspace_root

setup_script=\${BASH_SOURCE:-\$0}
case "\$setup_script" in
    */*) setup_dir=\$(CDPATH= cd -- "\$(dirname -- "\$setup_script")" && pwd) ;;
    *) setup_dir=\$(pwd) ;;
esac

BUNDLE_ROOT="\$setup_dir"
ARTIFACT_ARCHIVE=\$(find "\$BUNDLE_ROOT/artifacts" -type f -name '*.tar.gz' | sort | sed -n '1p')
[ -n "\$ARTIFACT_ARCHIVE" ] || { echo "setup-env: no package archive found under \$BUNDLE_ROOT/artifacts" >&2; return 1 2>/dev/null || exit 1; }

archive_count=\$(find "\$BUNDLE_ROOT/artifacts" -type f -name '*.tar.gz' | wc -l | tr -d ' ')
[ "\$archive_count" = "1" ] || { echo "setup-env: expected one package archive, found \$archive_count" >&2; return 1 2>/dev/null || exit 1; }

EXTRACT_DIR="\$BUNDLE_ROOT/extracted"
mkdir -p "\$EXTRACT_DIR"

if command -v sha256sum >/dev/null 2>&1; then
    checksum_file=\$(find "\$BUNDLE_ROOT/checksums" -type f -name '*.sha256' | sort | sed -n '1p')
    if [ -n "\$checksum_file" ]; then
        (cd "\$(dirname -- "\$ARTIFACT_ARCHIVE")" && sha256sum -c "\$checksum_file" >/dev/null)
    fi
fi

package_dir=\$(tar -tzf "\$ARTIFACT_ARCHIVE" | sed -n '1s#/.*##p')
[ -n "\$package_dir" ] || { echo "setup-env: package archive is empty: \$ARTIFACT_ARCHIVE" >&2; return 1 2>/dev/null || exit 1; }

if [ ! -d "\$EXTRACT_DIR/\$package_dir" ]; then
    tar -xzf "\$ARTIFACT_ARCHIVE" -C "\$EXTRACT_DIR"
fi

BIN="\$EXTRACT_DIR/\$package_dir/\$package_dir"
[ -x "\$BIN" ] || { echo "setup-env: binary not found or not executable: \$BIN" >&2; return 1 2>/dev/null || exit 1; }

if [ -z "\${CFG:-}" ]; then
    if [ -n "\$DEFAULT_CFG" ]; then
        CFG="\$DEFAULT_CFG"
    else
        CFG="\$HOME/.config/duplicacy-backup"
    fi
fi

if [ -z "\${SEC:-}" ]; then
    if [ -n "\$DEFAULT_SEC" ]; then
        SEC="\$DEFAULT_SEC"
    else
        SEC="\$CFG/secrets"
    fi
fi

LABEL="\${LABEL:-\$DEFAULT_LABEL}"
TARGET="\${TARGET:-\$DEFAULT_TARGET}"
WORKSPACE_ROOT="\${WORKSPACE_ROOT:-\$DEFAULT_WORKSPACE_ROOT}"
RESTORE_ROOT="\${RESTORE_ROOT:-\$WORKSPACE_ROOT}"

export BUNDLE_ROOT BIN CFG SEC LABEL TARGET WORKSPACE_ROOT RESTORE_ROOT

cat <<SETUP_EOF
Smoke bundle environment:
  BUNDLE_ROOT     : \$BUNDLE_ROOT
  BIN             : \$BIN
  CFG             : \$CFG
  SEC             : \$SEC
  LABEL           : \$LABEL
  TARGET          : \$TARGET
  WORKSPACE_ROOT  : \$WORKSPACE_ROOT
SETUP_EOF
EOF
chmod 755 "$bundle_dir/setup-env.sh"

cat > "$bundle_dir/README.md" <<EOF
# $RUN_ID

Start with:

\`\`\`bash
. ./setup-env.sh
less instructions/$instructions_name
\`\`\`

Contents:

- \`setup-env.sh\` extracts the package and exports \`BIN\`, \`CFG\`, \`SEC\`, \`LABEL\`, \`TARGET\`, and \`WORKSPACE_ROOT\`.
- \`artifacts/$artifact_platform/\` contains the Linux package tarball.
- \`checksums/$artifact_platform/\` contains the package checksum.
- \`instructions/\` contains the NAS smoke-test procedure.
- \`metadata/build.json\` records the exact build, commit, platform, and package details.
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
