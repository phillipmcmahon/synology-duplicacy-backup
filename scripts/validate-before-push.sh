#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WITH_UI_SMOKE=0

usage() {
    cat <<'EOF'
Usage: ./scripts/validate-before-push.sh [OPTIONS]

Run the local validation gate that mirrors GitHub's lint and test jobs before
pushing to origin.

Options:
  --with-ui-smoke   Also build and verify the UI surface smoke bundle
  --help            Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --with-ui-smoke)
            WITH_UI_SMOKE=1
            shift
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

cd "$ROOT"

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
    echo "Go files are not formatted. Run gofmt -w on these files:" >&2
    printf '%s\n' "$unformatted" >&2
    exit 1
fi

echo "go vet ./..."
go vet ./...

echo "go run honnef.co/go/tools/cmd/staticcheck ./..."
go run honnef.co/go/tools/cmd/staticcheck ./...

echo "Plan section boundary check"
sh scripts/check-plan-section-boundary.sh

echo "go test -v -race ./..."
go test -v -race ./...

echo "script tests"
for test_script in scripts/test-*.sh; do
    sh "$test_script"
done

if [ "$WITH_UI_SMOKE" -eq 1 ]; then
    echo "UI surface CI smoke"
    sh scripts/ci-smoke-ui-surface.sh
fi

echo "local pre-push validation passed"
