#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
MIN_COVERAGE="${MIN_COVERAGE:-85.0}"
PACKAGE_COVERAGE_FLOORS="${PACKAGE_COVERAGE_FLOORS-internal/secrets=93.1 internal/exec=95.2 internal/duplicacy=89.7}"
INPUT=""
OVERALL=""

usage() {
    cat <<'EOF'
Usage: ./scripts/check-coverage-floor.sh [OPTIONS]

Run Go coverage and fail if any package, or the aggregate total, is below the
configured minimum coverage floor. Packages listed in PACKAGE_COVERAGE_FLOORS
use their stricter package-specific floor.

Options:
  --input <path>     Parse an existing go test -cover output file
  --overall <pct>    Overall coverage value to validate with --input
  --help             Show this help text

Environment:
  MIN_COVERAGE              Default coverage floor percentage (default: 85.0)
  PACKAGE_COVERAGE_FLOORS   Whitespace-separated package=floor entries. Package
                            names may be full import paths or repo-relative
                            paths such as internal/secrets.
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
awk -v min="$MIN_COVERAGE" -v floors="$PACKAGE_COVERAGE_FLOORS" '
    function trim(value) {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        return value
    }
    function package_matches(pkg, floor_pkg) {
        return pkg == floor_pkg || substr(pkg, length(pkg) - length(floor_pkg)) == "/" floor_pkg
    }
    function package_floor(pkg,    i) {
        for (i = 1; i <= floor_count; i++) {
            if (package_matches(pkg, floor_pkg[i])) {
                floor_seen[i] = 1
                return floor_value[i]
            }
        }
        return min
    }
    BEGIN {
        floor_count = split(floors, floor_lines, /[[:space:]]+/)
        parsed_floor_count = 0
        for (i = 1; i <= floor_count; i++) {
            line = trim(floor_lines[i])
            if (line == "") {
                continue
            }
            separator = index(line, "=")
            if (separator == 0) {
                printf "invalid package-specific floor entry: %s\n", line
                invalid = 1
                continue
            }
            parsed_floor_count++
            floor_pkg[parsed_floor_count] = trim(substr(line, 1, separator - 1))
            floor_value[parsed_floor_count] = trim(substr(line, separator + 1))
        }
        floor_count = parsed_floor_count
    }
    /coverage:/ {
        package_name = $1
        if ($1 == "ok" || $1 == "?") {
            package_name = $2
        }
        pct = $0
        sub(/.*coverage: /, "", pct)
        sub(/%.*/, "", pct)
        seen = 1
        floor = package_floor(package_name)
        if ((pct + 0) < (floor + 0)) {
            printf "%s: %s%% below %.1f%%\n", package_name, pct, floor
            failed = 1
        }
    }
    END {
        for (i = 1; i <= floor_count; i++) {
            if (!floor_seen[i]) {
                printf "stale package-specific floor: %s=%s\n", floor_pkg[i], floor_value[i]
                failed = 1
            }
        }
        if (invalid) {
            failed = 1
        }
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
if [ -n "$OVERALL" ]; then
    if awk -v pct="$OVERALL" -v min="$MIN_COVERAGE" 'BEGIN { exit !((pct + 0) < (min + 0)) }'; then
        {
            echo "Coverage floor check failed: overall coverage ${OVERALL}% is below ${MIN_COVERAGE}%."
        } >>"$tmp_failures"
        status=1
    fi
fi

if [ "$status" -ne 0 ]; then
    echo "Coverage floor check failed:" >&2
    cat "$tmp_failures" >&2
    exit 1
fi

if [ -n "$OVERALL" ]; then
    echo "Coverage floor check passed: all packages and overall coverage are at or above their configured floors (default ${MIN_COVERAGE}%, overall ${OVERALL}%)."
else
    echo "Coverage floor check passed: all reported packages are at or above their configured floors (default ${MIN_COVERAGE}%)."
fi
