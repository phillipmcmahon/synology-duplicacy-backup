#!/bin/sh

set -eu

ROOT="$(CDPATH= cd "$(dirname "$0")/.." && pwd)"
TMP_DIR="$(mktemp -d)"

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

fail() {
    echo "Error: $*" >&2
    exit 1
}

assert_contains() {
    file="$1"
    pattern="$2"
    grep -F -- "$pattern" "$file" >/dev/null || {
        echo "Expected to find: $pattern" >&2
        echo "Actual output:" >&2
        sed -n '1,160p' "$file" >&2
        exit 1
    }
}

cat >"$TMP_DIR/pass.json" <<'EOF'
{
  "items": [
    {
      "id": "item1",
      "number": 1,
      "title": "Closed clean",
      "state": "CLOSED",
      "labels": ["story"],
      "status": "Done",
      "workflow": "Done"
    },
    {
      "id": "item2",
      "number": 2,
      "title": "Review clean",
      "state": "OPEN",
      "labels": ["story", "status:review"],
      "status": "In Progress",
      "workflow": "Review"
    },
    {
      "id": "item3",
      "number": 3,
      "title": "Open completed epic",
      "state": "OPEN",
      "labels": ["epic"],
      "status": "Done",
      "workflow": "Done"
    }
  ]
}
EOF

cat >"$TMP_DIR/fail.json" <<'EOF'
{
  "items": [
    {
      "id": "item1",
      "number": 10,
      "title": "Closed but still review",
      "state": "CLOSED",
      "labels": ["story", "status:review"],
      "status": "Done",
      "workflow": "Review"
    },
    {
      "id": "item2",
      "number": 11,
      "title": "Open done story",
      "state": "OPEN",
      "labels": ["story"],
      "status": "Done",
      "workflow": "Done"
    },
    {
      "id": "item3",
      "number": 12,
      "title": "Mismatched in progress label",
      "state": "OPEN",
      "labels": ["story", "status:in-progress"],
      "status": "Todo",
      "workflow": "Ready"
    }
  ]
}
EOF

sh "$ROOT/scripts/project-board-audit.sh" --input-json "$TMP_DIR/pass.json" >"$TMP_DIR/pass.out"
assert_contains "$TMP_DIR/pass.out" "Project board audit passed"

if sh "$ROOT/scripts/project-board-audit.sh" --input-json "$TMP_DIR/fail.json" >"$TMP_DIR/fail.out" 2>&1; then
    fail "project-board-audit should fail for inconsistent board data"
fi

assert_contains "$TMP_DIR/fail.out" "closed-not-done"
assert_contains "$TMP_DIR/fail.out" "closed-has-status-label"
assert_contains "$TMP_DIR/fail.out" "open-non-epic-done"
assert_contains "$TMP_DIR/fail.out" "label-field-mismatch"

echo "project-board-audit script tests passed"
