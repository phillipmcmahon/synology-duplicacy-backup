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

docker run --rm \
    -v "$ROOT":/work \
    -w /work \
    golang:1.26 \
    /bin/sh -lc '
        set -eu
        export DEBIAN_FRONTEND=noninteractive
        export PATH="/usr/local/go/bin:$PATH"
        apt-get update >/dev/null
        apt-get install -y --no-install-recommends file >/dev/null
        sh ./scripts/package-linux-artifact.sh --repo-root /work "$@"
    ' -- "$@"
