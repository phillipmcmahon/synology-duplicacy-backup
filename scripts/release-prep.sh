#!/bin/sh

set -eu

VERSION=""
DOCKER_BIN="${DOCKER_BIN:-docker}"

usage() {
    cat <<'EOF'
Usage: ./scripts/release-prep.sh [OPTIONS]

Run the standard release-prep validation flow from a clean main tree, capture
Linux Go 1.26 validation outputs, and generate a draft GitHub release-notes
file with the measured coverage numbers.

Options:
  --version <value>   Required release version without leading v (for example 2.1.7)
  --help              Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

script_dir() {
    CDPATH= cd -- "$(dirname -- "$0")" && pwd
}

repo_root() {
    dirname "$(script_dir)"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version)
            [ "$#" -ge 2 ] || fail "--version requires a value"
            VERSION="$2"
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

ROOT="$(repo_root)"
require_command git
require_command "$DOCKER_BIN"
require_command awk
require_command grep
require_command sed
require_command tail
require_command mktemp

BRANCH="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD)"
[ "$BRANCH" = "main" ] || fail "release prep must run from branch 'main' (current: $BRANCH)"

STATUS="$(git -C "$ROOT" status --porcelain)"
[ -z "$STATUS" ] || fail "release prep requires a clean git tree"

[ -n "$VERSION" ] || fail "release prep requires --version <value>; use make release-prep RELEASE_VERSION=x.y.z"
[ "${VERSION#v}" != "$VERSION" ] && VERSION="${VERSION#v}"
[ -n "$VERSION" ] || fail "could not determine release version"

CHANGELOG_TAG="## [v$VERSION]"
grep -F "$CHANGELOG_TAG" "$ROOT/CHANGELOG.md" >/dev/null || fail "CHANGELOG.md does not contain $CHANGELOG_TAG"
grep -F 'version   = "dev"' "$ROOT/cmd/duplicacy-backup/main.go" >/dev/null || fail "main.go version fallback must remain dev; release builds inject main.version via ldflags"

PREP_DIR="$ROOT/build/release-prep/v$VERSION"
mkdir -p "$PREP_DIR"

TEST_OUT="$PREP_DIR/go-test.txt"
VET_OUT="$PREP_DIR/go-vet.txt"
STATICCHECK_OUT="$PREP_DIR/staticcheck.txt"
COVER_OUT="$PREP_DIR/go-cover.txt"
SUMMARY_OUT="$PREP_DIR/coverage-summary.txt"
NOTES_OUT="$PREP_DIR/release-notes.md"

run_in_linux() {
    "$DOCKER_BIN" run --rm \
        -v "$ROOT":/work \
        -w /work \
        golang:1.26 \
        /bin/sh -lc "$1"
}

COMMON_EXPORTS='set -eu; export PATH=/usr/local/go/bin:$PATH; export GOCACHE=/tmp/gocache;'

run_in_linux "$COMMON_EXPORTS go test ./..." >"$TEST_OUT" 2>&1
run_in_linux "$COMMON_EXPORTS go vet ./..." >"$VET_OUT" 2>&1
run_in_linux "$COMMON_EXPORTS go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./..." >"$STATICCHECK_OUT" 2>&1
run_in_linux "$COMMON_EXPORTS go test -cover ./..." >"$COVER_OUT" 2>&1
run_in_linux "$COMMON_EXPORTS go test -coverprofile=/tmp/cover.out ./... >/tmp/cover.txt 2>/dev/null; go tool cover -func=/tmp/cover.out | tail -n 1" >"$SUMMARY_OUT" 2>&1
grep '^total:' "$SUMMARY_OUT" >"$SUMMARY_OUT.tmp"
mv "$SUMMARY_OUT.tmp" "$SUMMARY_OUT"

extract_pkg_coverage() {
    pkg="$1"
    line="$(grep -F "github.com/phillipmcmahon/synology-duplicacy-backup/$pkg" "$COVER_OUT" | tail -n 1 || true)"
    [ -n "$line" ] || {
        printf 'unknown'
        return 0
    }
    printf '%s' "$line" | awk -F'coverage: ' '{print $2}' | awk '{print $1}'
}

OVERALL_COVERAGE="$(awk '{print $3}' "$SUMMARY_OUT" | tail -n 1)"
WORKFLOW_COVERAGE="$(extract_pkg_coverage internal/workflow)"
CMD_COVERAGE="$(extract_pkg_coverage cmd/duplicacy-backup)"
DUPLICACY_COVERAGE="$(extract_pkg_coverage internal/duplicacy)"
EXEC_COVERAGE="$(extract_pkg_coverage internal/exec)"
SECRETS_COVERAGE="$(extract_pkg_coverage internal/secrets)"

cat >"$NOTES_OUT" <<EOF
## Highlights
- Replace these bullets with the shipped user-visible changes.
- Fold in any important notes from superseded release attempts before publishing.

## Validation
- Linux Go 1.26: \`go test ./...\`
- Linux Go 1.26: \`go vet ./...\`
- Linux Go 1.26: \`go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...\`
- Linux Go 1.26: \`go test -cover ./...\`

## Coverage
- Linux Go 1.26: overall coverage = \`$OVERALL_COVERAGE\`
- Linux Go 1.26: \`internal/workflow\` coverage = \`$WORKFLOW_COVERAGE\`
- Linux Go 1.26: \`cmd/duplicacy-backup\` coverage = \`$CMD_COVERAGE\`
- Linux Go 1.26: \`internal/duplicacy\` coverage = \`$DUPLICACY_COVERAGE\`
- Linux Go 1.26: \`internal/exec\` coverage = \`$EXEC_COVERAGE\`
- Linux Go 1.26: \`internal/secrets\` coverage = \`$SECRETS_COVERAGE\`
EOF

cat <<EOF
Release prep complete for v$VERSION

Artifacts written to:
  $PREP_DIR

Validation logs:
  $TEST_OUT
  $VET_OUT
  $STATICCHECK_OUT
  $COVER_OUT
  $SUMMARY_OUT

Draft release notes:
  $NOTES_OUT
EOF
