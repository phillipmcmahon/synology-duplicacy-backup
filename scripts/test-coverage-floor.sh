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

MIN_COVERAGE=85.0 sh "$SCRIPT" --input "$pass_output" --overall 85.0 >/dev/null

package_fail_output="$TMP_DIR/package-fail.txt"
cat >"$package_fail_output" <<'EOF'
ok  	example.com/project/cmd/tool	0.123s	coverage: 84.9% of statements
ok  	example.com/project/internal/core	0.123s	coverage: 92.4% of statements
EOF

if MIN_COVERAGE=85.0 sh "$SCRIPT" --input "$package_fail_output" --overall 90.0 >"$TMP_DIR/package-fail.out" 2>&1; then
    echo "expected package coverage failure" >&2
    exit 1
fi
if ! grep -F "example.com/project/cmd/tool: 84.9%" "$TMP_DIR/package-fail.out" >/dev/null; then
    echo "package failure output did not name failing package" >&2
    cat "$TMP_DIR/package-fail.out" >&2
    exit 1
fi

overall_fail_output="$TMP_DIR/overall-fail.txt"
cat >"$overall_fail_output" <<'EOF'
ok  	example.com/project/cmd/tool	0.123s	coverage: 85.1% of statements
ok  	example.com/project/internal/core	0.123s	coverage: 92.4% of statements
EOF

if MIN_COVERAGE=85.0 sh "$SCRIPT" --input "$overall_fail_output" --overall 84.9 >"$TMP_DIR/overall-fail.out" 2>&1; then
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

if MIN_COVERAGE=85.0 sh "$SCRIPT" --input "$bare_package_output" >"$TMP_DIR/bare-package.out" 2>&1; then
    echo "expected bare package coverage failure" >&2
    exit 1
fi
if ! grep -F "example.com/project/internal/generated: 0.0%" "$TMP_DIR/bare-package.out" >/dev/null; then
    echo "bare package failure output did not parse package name" >&2
    cat "$TMP_DIR/bare-package.out" >&2
    exit 1
fi

echo "coverage floor script tests passed"
