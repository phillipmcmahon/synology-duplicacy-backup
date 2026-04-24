#!/bin/sh

set -eu

VERSION=""
BUILD_TIME=""
GOOS=""
GOARCH=""
GOARM=""
OUTPUT_DIR=""
REPO_ROOT=""

usage() {
    cat <<'EOF'
Usage: ./scripts/package-linux-artifact.sh [OPTIONS]

Build, package, checksum, and smoke-test one Linux artifact inside a Linux
environment.

Options:
  --version <value>      Version string embedded into the binary
  --build-time <value>   Build timestamp in UTC RFC3339 form
  --goos <value>         Target GOOS
  --goarch <value>       Target GOARCH
  --goarm <value>        Target GOARM (for GOARCH=arm)
  --output-dir <path>    Directory for the final tarball and checksum
  --repo-root <path>     Repository root (default: script parent directory)
  --help                 Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_linux_host() {
    host_os="$(uname -s)"
    [ "$host_os" = "Linux" ] || fail "package-linux-artifact.sh must run inside Linux; use scripts/package-linux-docker.sh from macOS"
}

resolve_go_command() {
    if command -v go >/dev/null 2>&1; then
        command -v go
        return 0
    fi
    if [ -x /usr/local/go/bin/go ]; then
        printf '/usr/local/go/bin/go\n'
        return 0
    fi
    fail "required command not found: go"
}

script_dir() {
    CDPATH= cd -- "$(dirname -- "$0")" && pwd
}

repo_root_default() {
    dirname "$(script_dir)"
}

native_goarch() {
    case "$(uname -m)" in
        x86_64|amd64)
            printf 'amd64\n'
            ;;
        aarch64|arm64)
            printf 'arm64\n'
            ;;
        armv7l|armv7)
            printf 'arm\n'
            ;;
        *)
            printf 'unknown\n'
            ;;
    esac
}

expected_file_pattern() {
    case "$1/$2" in
        linux/amd64)
            printf 'ELF 64-bit LSB executable, x86-64\n'
            ;;
        linux/arm64)
            printf 'ELF 64-bit LSB executable, ARM aarch64\n'
            ;;
        linux/arm)
            printf 'ELF 32-bit LSB executable, ARM\n'
            ;;
        *)
            fail "no file(1) expectation defined for target $1/$2"
            ;;
    esac
}

arch_suffix() {
    if [ "$GOARCH" = "arm" ] && [ -n "$GOARM" ]; then
        printf '%sv%s\n' "$GOARCH" "$GOARM"
        return 0
    fi
    printf '%s\n' "$GOARCH"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
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
        --output-dir)
            [ "$#" -ge 2 ] || fail "--output-dir requires a value"
            OUTPUT_DIR="$2"
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

[ -n "$VERSION" ] || fail "--version is required"
[ -n "$BUILD_TIME" ] || fail "--build-time is required"
[ -n "$GOOS" ] || fail "--goos is required"
[ -n "$GOARCH" ] || fail "--goarch is required"

REPO_ROOT="${REPO_ROOT:-$(repo_root_default)}"
OUTPUT_DIR="${OUTPUT_DIR:-$REPO_ROOT/build/test-packages/release}"

require_linux_host
require_command tar
require_command sha256sum
require_command file
require_command mktemp
require_command grep

GO_CMD="$(resolve_go_command)"

[ -f "$REPO_ROOT/README.md" ] || fail "repo root not valid: $REPO_ROOT"
[ -f "$REPO_ROOT/scripts/install-synology.sh" ] || fail "installer script not found"

ARCH_SUFFIX="$(arch_suffix)"
BASENAME="duplicacy-backup_${VERSION}_${GOOS}_${ARCH_SUFFIX}"
BINARY_PATH="$OUTPUT_DIR/$BASENAME"
ARCHIVE_PATH="$OUTPUT_DIR/$BASENAME.tar.gz"
CHECKSUM_PATH="$OUTPUT_DIR/$BASENAME.tar.gz.sha256"

mkdir -p "$OUTPUT_DIR"

[ ! -e "$BINARY_PATH" ] || fail "output file already exists: $BINARY_PATH"
[ ! -e "$ARCHIVE_PATH" ] || fail "output archive already exists: $ARCHIVE_PATH"
[ ! -e "$CHECKSUM_PATH" ] || fail "output checksum already exists: $CHECKSUM_PATH"

PKG_DIR="$(mktemp -d)"
EXTRACT_DIR="$(mktemp -d)"
trap 'rm -rf "$PKG_DIR" "$EXTRACT_DIR" "$BINARY_PATH"' EXIT

(
    cd "$REPO_ROOT"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOARM="$GOARM" \
        "$GO_CMD" build \
        -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME" \
        -o "$BINARY_PATH" \
        ./cmd/duplicacy-backup
)

PACKAGE_ROOT="$PKG_DIR/$BASENAME"
mkdir -p "$PACKAGE_ROOT"
cp "$BINARY_PATH" "$PACKAGE_ROOT/"
cp "$REPO_ROOT/scripts/install-synology.sh" "$PACKAGE_ROOT/install.sh"
cp "$REPO_ROOT/README.md" "$REPO_ROOT/LICENSE" "$PACKAGE_ROOT/"
chmod 755 "$PACKAGE_ROOT/install.sh"

tar -czf "$ARCHIVE_PATH" -C "$PKG_DIR" "$BASENAME"
(
    cd "$OUTPUT_DIR"
    sha256sum "$BASENAME.tar.gz" > "$BASENAME.tar.gz.sha256"
    sha256sum -c "$BASENAME.tar.gz.sha256" >/dev/null
)

tar -xzf "$ARCHIVE_PATH" -C "$EXTRACT_DIR"

EXTRACTED_ROOT="$EXTRACT_DIR/$BASENAME"
EXTRACTED_BINARY="$EXTRACTED_ROOT/$BASENAME"
EXTRACTED_INSTALLER="$EXTRACTED_ROOT/install.sh"

[ -d "$EXTRACTED_ROOT" ] || fail "packaged root missing from archive"
[ -f "$EXTRACTED_ROOT/README.md" ] || fail "README.md missing from archive"
[ -f "$EXTRACTED_ROOT/LICENSE" ] || fail "LICENSE missing from archive"
[ -x "$EXTRACTED_BINARY" ] || fail "binary missing or not executable in archive"
[ -x "$EXTRACTED_INSTALLER" ] || fail "install.sh missing or not executable in archive"

EXPECTED_PATTERN="$(expected_file_pattern "$GOOS" "$GOARCH")"
FILE_OUTPUT="$(file "$EXTRACTED_BINARY")"
printf '%s\n' "$FILE_OUTPUT" | grep -F "$EXPECTED_PATTERN" >/dev/null || fail "binary architecture mismatch: $FILE_OUTPUT"

NATIVE_GOARCH="$(native_goarch)"
if [ "$GOOS" = "linux" ] && [ "$GOARCH" = "$NATIVE_GOARCH" ] && [ "$GOARCH" != "arm" ]; then
    "$EXTRACTED_BINARY" --version | grep -F "$VERSION" >/dev/null || fail "binary --version output did not include $VERSION"
    "$EXTRACTED_BINARY" --help >/dev/null
else
    echo "Skipping binary execution smoke test for non-native target $GOOS/$GOARCH" >&2
fi

sh "$EXTRACTED_INSTALLER" --help >/dev/null

printf '%s\n' "$BASENAME"
