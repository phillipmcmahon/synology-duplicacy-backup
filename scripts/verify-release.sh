#!/bin/sh

set -eu

REPO="phillipmcmahon/synology-duplicacy-backup"
HOST="homestorage"
REMOTE_ROOT="/volume1/homes/phillipmcmahon/code/duplicacy-backup"
TAG=""
VERIFY_ATTESTATIONS=1

usage() {
    cat <<'EOF'
Usage: ./scripts/verify-release.sh --tag <value> [OPTIONS]

Verify one published GitHub release and its latest mirrored artefacts on homestorage.

Checks:
  - GitHub release exists and is neither draft nor prerelease
  - release name and tag match the requested tag
  - release notes include Highlights, Validation, and Coverage sections
  - expected packaged assets are present on GitHub
  - the GitHub release has a release attestation
  - release assets verify against the release attestation
  - the remote tag commit matches the local tag commit
  - the mirrored artefact set exists under homestorage latest/<tag>

Options:
  --tag <value>         Release tag to verify (for example v4.1.4)
  --repo <value>        GitHub repository in owner/name form
                        (default: phillipmcmahon/synology-duplicacy-backup)
  --host <value>        SSH host used for the mirror check (default: homestorage)
  --remote-root <path>  Remote base directory
                        (default: /volume1/homes/phillipmcmahon/code/duplicacy-backup)
  --skip-attestations   Skip GitHub release and asset attestation checks
                        (only for historical releases)
  --help                Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --tag)
            [ "$#" -ge 2 ] || fail "--tag requires a value"
            TAG="$2"
            shift 2
            ;;
        --repo)
            [ "$#" -ge 2 ] || fail "--repo requires a value"
            REPO="$2"
            shift 2
            ;;
        --host)
            [ "$#" -ge 2 ] || fail "--host requires a value"
            HOST="$2"
            shift 2
            ;;
        --remote-root)
            [ "$#" -ge 2 ] || fail "--remote-root requires a value"
            REMOTE_ROOT="$2"
            shift 2
            ;;
        --skip-attestations)
            VERIFY_ATTESTATIONS=0
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

[ -n "$TAG" ] || fail "--tag is required"

require_command gh
require_command git
require_command jq
require_command ssh
require_command sort
require_command mktemp
require_command diff

VERSION="${TAG#v}"
REMOTE_DIR="$REMOTE_ROOT/latest/$TAG"

EXPECTED_RELEASE_ASSETS=$(cat <<EOF
SHA256SUMS.txt
duplicacy-backup_${VERSION}_linux_amd64.tar.gz
duplicacy-backup_${VERSION}_linux_amd64.tar.gz.sha256
duplicacy-backup_${VERSION}_linux_arm64.tar.gz
duplicacy-backup_${VERSION}_linux_arm64.tar.gz.sha256
duplicacy-backup_${VERSION}_linux_armv7.tar.gz
duplicacy-backup_${VERSION}_linux_armv7.tar.gz.sha256
EOF
)

EXPECTED_MIRROR_FILES=$(cat <<EOF
SHA256SUMS.txt
Source code (tar.gz)
Source code (zip)
duplicacy-backup_${VERSION}_linux_amd64.tar.gz
duplicacy-backup_${VERSION}_linux_amd64.tar.gz.sha256
duplicacy-backup_${VERSION}_linux_arm64.tar.gz
duplicacy-backup_${VERSION}_linux_arm64.tar.gz.sha256
duplicacy-backup_${VERSION}_linux_armv7.tar.gz
duplicacy-backup_${VERSION}_linux_armv7.tar.gz.sha256
EOF
)

release_json_file="$(mktemp)"
release_assets_tmp="$(mktemp)"
mirror_assets_tmp="$(mktemp)"
expected_release_tmp="$(mktemp)"
expected_mirror_tmp="$(mktemp)"
attestation_dir="$(mktemp -d)"
cleanup() {
    rm -f "$release_json_file" "$release_assets_tmp" "$mirror_assets_tmp" "$expected_release_tmp" "$expected_mirror_tmp"
    rm -rf "$attestation_dir"
}
trap cleanup EXIT INT TERM

gh release view "$TAG" --repo "$REPO" --json tagName,name,body,isDraft,isPrerelease,assets,url >"$release_json_file"

jq -e '.isDraft == false and .isPrerelease == false' "$release_json_file" >/dev/null || fail "release is draft or prerelease"
jq -e --arg tag "$TAG" '.tagName == $tag and .name == $tag' "$release_json_file" >/dev/null || fail "release tag/name do not match $TAG"

jq -e '.body | contains("## Highlights")' "$release_json_file" >/dev/null || fail "release notes missing Highlights section"
jq -e '.body | contains("## Validation")' "$release_json_file" >/dev/null || fail "release notes missing Validation section"
jq -e '.body | contains("## Coverage")' "$release_json_file" >/dev/null || fail "release notes missing Coverage section"

jq -r '.assets[].name' "$release_json_file" | sort >"$release_assets_tmp"
printf '%s\n' "$EXPECTED_RELEASE_ASSETS" | sort >"$expected_release_tmp"
diff -u "$expected_release_tmp" "$release_assets_tmp" >/dev/null || fail "release asset set does not match expected names"

if [ "$VERIFY_ATTESTATIONS" -eq 1 ]; then
    gh release verify "$TAG" --repo "$REPO" >/dev/null || fail "GitHub release attestation verification failed for $TAG"

    printf '%s\n' "$EXPECTED_RELEASE_ASSETS" | while IFS= read -r asset; do
        [ -n "$asset" ] || continue
        gh release download "$TAG" --repo "$REPO" --pattern "$asset" --dir "$attestation_dir" >/dev/null
        gh release verify-asset "$TAG" "$attestation_dir/$asset" --repo "$REPO" >/dev/null || fail "release asset attestation verification failed for $asset"
    done
fi

local_commit="$(git -C "$(git rev-parse --show-toplevel)" rev-list -n 1 "$TAG")"
remote_commit="$(git ls-remote "https://github.com/$REPO.git" "refs/tags/$TAG^{}" | awk '{print $1}')"
[ -n "$remote_commit" ] || fail "could not resolve remote tag commit for $TAG"
[ "$local_commit" = "$remote_commit" ] || fail "remote tag commit does not match local tag commit"

ssh "$HOST" "test -d '$REMOTE_DIR'" || fail "remote mirror directory does not exist: $HOST:$REMOTE_DIR"
ssh "$HOST" "find '$REMOTE_DIR' -maxdepth 1 -type f -exec basename {} \;" | sort >"$mirror_assets_tmp"
printf '%s\n' "$EXPECTED_MIRROR_FILES" | sort >"$expected_mirror_tmp"
diff -u "$expected_mirror_tmp" "$mirror_assets_tmp" >/dev/null || fail "mirrored artefact set does not match expected names"

echo "Verified $TAG"
echo "Release URL: $(jq -r '.url' "$release_json_file")"
echo "Remote mirror: $HOST:$REMOTE_DIR"
if [ "$VERIFY_ATTESTATIONS" -eq 1 ]; then
    echo "Release attestations: verified"
else
    echo "Release attestations: skipped"
fi
