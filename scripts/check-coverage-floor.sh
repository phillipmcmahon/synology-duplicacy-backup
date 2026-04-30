#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
MIN_COVERAGE="${MIN_COVERAGE:-85.0}"
INPUT=""
OVERALL=""

usage() {
    cat <<'EOF'
Usage: ./scripts/check-coverage-floor.sh [OPTIONS]

Run Go coverage and fail if any package, or the aggregate total, is below the
configured minimum coverage floor.

Options:
  --input <path>     Parse an existing go test -cover output file
  --overall <pct>    Overall coverage value to validate with --input
  --help             Show this help text

Environment:
  MIN_COVERAGE       Coverage floor percentage (default: 85.0)
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --input)
            [ "$#" -ge 2 ] || fail "--input requires a path"
            INPUT="$2"
            shift 2
            ;;
        --overall)
            [ "$#" -ge 2 ] || fail "--overall requires a percentage"
            OVERALL="$2"
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

tmp_output=""
tmp_profile=""
tmp_failures="$(mktemp)"
trap 'rm -f "$tmp_output" "$tmp_profile" "$tmp_failures"' EXIT

cd "$ROOT"

if [ -z "$INPUT" ]; then
    tmp_output="$(mktemp)"
    tmp_profile="$(mktemp)"
    echo "go test -coverprofile ./..."
    go test -coverprofile="$tmp_profile" ./... | tee "$tmp_output"
    INPUT="$tmp_output"
    OVERALL="$(go tool cover -func="$tmp_profile" | awk '/^total:/ { gsub("%", "", $3); print $3 }')"
fi

status=0
awk -v min="$MIN_COVERAGE" '
    /coverage:/ {
        package_name = $1
        if ($1 == "ok" || $1 == "?") {
            package_name = $2
        }
        pct = $0
        sub(/.*coverage: /, "", pct)
        sub(/%.*/, "", pct)
        seen = 1
        if ((pct + 0) < (min + 0)) {
            printf "%s: %s%%\n", package_name, pct
            failed = 1
        }
    }
    END {
        if (!seen) {
            print "no package coverage lines found"
            exit 2
        }
        exit failed
    }
' "$INPUT" >"$tmp_failures" || status=$?
status="${status:-0}"

if [ "$status" -eq 2 ]; then
    cat "$tmp_failures" >&2
    exit 1
fi
if [ "$status" -ne 0 ]; then
    echo "Coverage floor check failed: packages below ${MIN_COVERAGE}%:" >&2
    cat "$tmp_failures" >&2
    exit 1
fi

if [ -n "$OVERALL" ]; then
    if awk -v pct="$OVERALL" -v min="$MIN_COVERAGE" 'BEGIN { exit !((pct + 0) < (min + 0)) }'; then
        echo "Coverage floor check failed: overall coverage ${OVERALL}% is below ${MIN_COVERAGE}%." >&2
        exit 1
    fi
fi

if [ -n "$OVERALL" ]; then
    echo "Coverage floor check passed: all packages and overall coverage are at or above ${MIN_COVERAGE}% (overall ${OVERALL}%)."
else
    echo "Coverage floor check passed: all reported packages are at or above ${MIN_COVERAGE}%."
fi
