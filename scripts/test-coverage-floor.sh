#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
SCRIPT="$ROOT/scripts/check-coverage-floor.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass_output="$TMP_DIR/pass.txt"
cat >"$pass_output" <<'EOF'
ok  	example.com/project/cmd/tool	0.123s	coverage: 85.0% of statements
ok  	example.com/project/internal/core	0.123s	coverage: 92.4% of statements
?   	example.com/project/internal/docs	[no test files]
EOF

MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS= sh "$SCRIPT" --input "$pass_output" --overall 85.0 >/dev/null

package_fail_output="$TMP_DIR/package-fail.txt"
cat >"$package_fail_output" <<'EOF'
ok  	example.com/project/cmd/tool	0.123s	coverage: 84.9% of statements
ok  	example.com/project/internal/core	0.123s	coverage: 92.4% of statements
EOF

if MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS= sh "$SCRIPT" --input "$package_fail_output" --overall 90.0 >"$TMP_DIR/package-fail.out" 2>&1; then
    echo "expected package coverage failure" >&2
    exit 1
fi
if ! grep -F "example.com/project/cmd/tool: 84.9% below 85.0%" "$TMP_DIR/package-fail.out" >/dev/null; then
    echo "package failure output did not name failing package" >&2
    cat "$TMP_DIR/package-fail.out" >&2
    exit 1
fi

overall_fail_output="$TMP_DIR/overall-fail.txt"
cat >"$overall_fail_output" <<'EOF'
ok  	example.com/project/cmd/tool	0.123s	coverage: 85.1% of statements
ok  	example.com/project/internal/core	0.123s	coverage: 92.4% of statements
EOF

if MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS= sh "$SCRIPT" --input "$overall_fail_output" --overall 84.9 >"$TMP_DIR/overall-fail.out" 2>&1; then
    echo "expected overall coverage failure" >&2
    exit 1
fi
if ! grep -F "overall coverage 84.9% is below 85.0%" "$TMP_DIR/overall-fail.out" >/dev/null; then
    echo "overall failure output did not name failing total" >&2
    cat "$TMP_DIR/overall-fail.out" >&2
    exit 1
fi

bare_package_output="$TMP_DIR/bare-package.txt"
cat >"$bare_package_output" <<'EOF'
example.com/project/internal/generated		coverage: 0.0% of statements
EOF

if MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS= sh "$SCRIPT" --input "$bare_package_output" >"$TMP_DIR/bare-package.out" 2>&1; then
    echo "expected bare package coverage failure" >&2
    exit 1
fi
if ! grep -F "example.com/project/internal/generated: 0.0% below 85.0%" "$TMP_DIR/bare-package.out" >/dev/null; then
    echo "bare package failure output did not parse package name" >&2
    cat "$TMP_DIR/bare-package.out" >&2
    exit 1
fi

specific_floor_pass_output="$TMP_DIR/specific-floor-pass.txt"
cat >"$specific_floor_pass_output" <<'EOF'
ok  	example.com/project/internal/secrets	0.123s	coverage: 93.1% of statements
ok  	example.com/project/internal/newpkg	0.123s	coverage: 85.0% of statements
EOF

MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS="internal/secrets=93.1" \
    sh "$SCRIPT" --input "$specific_floor_pass_output" --overall 85.0 >/dev/null

specific_floor_fail_output="$TMP_DIR/specific-floor-fail.txt"
cat >"$specific_floor_fail_output" <<'EOF'
ok  	example.com/project/internal/secrets	0.123s	coverage: 93.0% of statements
ok  	example.com/project/internal/newpkg	0.123s	coverage: 85.0% of statements
EOF

if MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS="internal/secrets=93.1" \
    sh "$SCRIPT" --input "$specific_floor_fail_output" --overall 85.0 >"$TMP_DIR/specific-floor-fail.out" 2>&1; then
    echo "expected package-specific coverage failure" >&2
    exit 1
fi
if ! grep -F "example.com/project/internal/secrets: 93.0% below 93.1%" "$TMP_DIR/specific-floor-fail.out" >/dev/null; then
    echo "package-specific failure output did not name stricter floor" >&2
    cat "$TMP_DIR/specific-floor-fail.out" >&2
    exit 1
fi

stale_floor_output="$TMP_DIR/stale-floor.txt"
cat >"$stale_floor_output" <<'EOF'
ok  	example.com/project/internal/core	0.123s	coverage: 92.4% of statements
EOF

if MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS="internal/missing=90.0" \
    sh "$SCRIPT" --input "$stale_floor_output" --overall 92.4 >"$TMP_DIR/stale-floor.out" 2>&1; then
    echo "expected stale package-specific floor failure" >&2
    exit 1
fi
if ! grep -F "stale package-specific floor: internal/missing=90.0" "$TMP_DIR/stale-floor.out" >/dev/null; then
    echo "stale floor output did not name missing package" >&2
    cat "$TMP_DIR/stale-floor.out" >&2
    exit 1
fi

multi_fail_output="$TMP_DIR/multi-fail.txt"
cat >"$multi_fail_output" <<'EOF'
ok  	example.com/project/internal/secrets	0.123s	coverage: 92.0% of statements
ok  	example.com/project/internal/core	0.123s	coverage: 84.0% of statements
EOF

if MIN_COVERAGE=85.0 PACKAGE_COVERAGE_FLOORS="internal/secrets=93.1 internal/missing=90.0" \
    sh "$SCRIPT" --input "$multi_fail_output" --overall 84.5 >"$TMP_DIR/multi-fail.out" 2>&1; then
    echo "expected aggregated coverage failure" >&2
    exit 1
fi
for expected in \
    "example.com/project/internal/secrets: 92.0% below 93.1%" \
    "example.com/project/internal/core: 84.0% below 85.0%" \
    "stale package-specific floor: internal/missing=90.0" \
    "overall coverage 84.5% is below 85.0%"; do
    if ! grep -F "$expected" "$TMP_DIR/multi-fail.out" >/dev/null; then
        echo "aggregated failure output missing: $expected" >&2
        cat "$TMP_DIR/multi-fail.out" >&2
        exit 1
    fi
done

echo "coverage floor script tests passed"
