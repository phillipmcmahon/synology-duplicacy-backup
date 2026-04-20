#!/bin/sh

set -eu

script_dir() {
    CDPATH= cd -- "$(dirname -- "$0")" && pwd
}

repo_root() {
    dirname "$(script_dir)"
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "Error: required command not found: $1" >&2
        exit 1
    }
}

ROOT="$(repo_root)"
require_command docker
GO_CONTAINER_IMAGE="$(awk 'toupper($1) == "FROM" { print $2; exit }' "$ROOT/tools/release-validation/Dockerfile")"
[ -n "$GO_CONTAINER_IMAGE" ] || {
    echo "Error: could not read Go container image from tools/release-validation/Dockerfile" >&2
    exit 1
}

docker run --rm \
    -v "$ROOT":/work \
    -w /work \
    "$GO_CONTAINER_IMAGE" \
    /bin/sh -lc '
        set -eu
        export DEBIAN_FRONTEND=noninteractive
        export PATH="/usr/local/go/bin:$PATH"
        apt-get update >/dev/null
        apt-get install -y --no-install-recommends file >/dev/null
        sh ./scripts/package-linux-artifact.sh --repo-root /work "$@"
    ' -- "$@"
