# Linux Test and Packaging Environment

Use this document whenever you need a consistent Linux environment for:

- release validation
- coverage runs
- test-package generation

This repo does **not** treat the macOS host as release-representative.

## Standard Environment

Use:

- Docker
- the Go container image declared in
  [`tools/release-validation/Dockerfile`](../tools/release-validation/Dockerfile)
- the repo mounted at `/work`
- a temporary Go build cache inside the container

Treat this as the standard Linux environment for the project.
Dependabot monitors the Dockerfile so routine Go container image updates arrive
as normal dependency PRs.

For normal release work, prefer:

```bash
make release-prep RELEASE_VERSION=4.x.y
```

That command wraps the standard Linux validation flow for the requested release
version and writes the release prep outputs into `build/release-prep/`.

## Rules

- Run validation in Linux, not on the macOS host.
- Generate local test tarballs and checksums in Linux, not on the macOS host.
- Treat the macOS host as an orchestrator only.
- Use the same container image for testing and packaging unless there is a
  deliberate reason not to.
- Public release artefacts are built by GitHub Actions after the release tag is
  pushed.

## Prerequisites on macOS

Any Docker-compatible Linux runtime is fine, but it must expose a working
`docker` CLI on the host.

Typical local setup:

```bash
docker version
```

If you use Colima, confirm it is running:

```bash
colima status
```

If it is stopped:

```bash
colima start
```

## Standard Validation Commands

Run these from the repo root.

### Representative release validation

```bash
GO_CONTAINER_IMAGE="$(awk 'toupper($1) == "FROM" { print $2; exit }' tools/release-validation/Dockerfile)"

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test ./...'

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go vet ./...'
```

### Coverage run

```bash
GO_CONTAINER_IMAGE="$(awk 'toupper($1) == "FROM" { print $2; exit }' tools/release-validation/Dockerfile)"

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test -cover ./...'
```

### Aggregate coverage

```bash
GO_CONTAINER_IMAGE="$(awk 'toupper($1) == "FROM" { print $2; exit }' tools/release-validation/Dockerfile)"

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && \
   /usr/local/go/bin/go test -coverprofile=/tmp/cover.out ./... >/tmp/cover.txt && \
   /usr/local/go/bin/go tool cover -func=/tmp/cover.out | tail -n 1'
```

## Standard Packaging Commands

Use the repo scripts for local test-package builds. They already enforce the
Linux-only packaging flow. Keep all local test packages under
`build/test-packages`; do not create ad-hoc package directories elsewhere under
`build/`.

### One artefact

```bash
sh ./scripts/package-linux-docker.sh \
  --version "$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
  --build-time "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
  --goos linux \
  --goarch amd64 \
  --output-dir /work/build/test-packages
```

### Supported Synology package set

```bash
make package-synology
```

## What the Packaging Scripts Do

`scripts/package-linux-docker.sh`:

- starts the Linux container
- mounts the repo at `/work`
- installs `file` inside the container
- delegates to the in-container packaging script

`scripts/package-linux-artifact.sh`:

- builds the target binary
- creates the tarball
- generates the `.sha256` file
- verifies checksum validity
- unpacks the archive and checks contents
- verifies binary architecture with `file`
- runs safe smoke checks such as:
  - binary `--version`
  - binary `--help`
  - installer `--help`

## Do Not Do These

- Do not build a local Linux tarball on macOS.
- Do not generate checksums on macOS for local Linux test artefacts.
- Do not trust a host-side smoke test for a Linux binary.
- Do not mix ad-hoc packaging commands with the standard scripts unless you are
  actively fixing the packaging scripts themselves.

## If Something Fails

Check these first:

- `docker version`
- enough disk space for the container image and Go cache
- the repo is mounted at `/work`
- `GOCACHE` is set inside the container
- the target files do not already exist in `build/test-packages`

If packaging still fails:

- fix the repo scripts
- rerun the same standard commands
- do not switch to manual macOS packaging as a workaround
