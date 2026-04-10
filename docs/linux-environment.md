# Linux Test and Packaging Environment

Use this document whenever you need a consistent Linux environment for:

- release validation
- coverage runs
- test-package generation
- final release-package generation

This repo does **not** treat the macOS host as release-representative.

## Standard Environment

Use:

- Docker
- the official `golang:1.26` container image
- the repo mounted at `/work`
- a temporary Go build cache inside the container

This is the standard Linux machine for this project.

For normal release work, prefer:

```bash
make release-prep
```

That command wraps the standard Linux validation flow and writes the release
prep outputs into `build/release-prep/`.

## Rules

- Run validation in Linux, not on the macOS host.
- Generate tarballs and checksums in Linux, not on the macOS host.
- Treat the macOS host as an orchestrator only.
- Use the same container image for testing and packaging unless there is a
  deliberate reason not to.

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
docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test ./...'

docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go vet ./...'
```

### Coverage run

```bash
docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test -cover ./...'
```

### Aggregate coverage

```bash
docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && \
   /usr/local/go/bin/go test -coverprofile=/tmp/cover.out ./... >/tmp/cover.txt && \
   /usr/local/go/bin/go tool cover -func=/tmp/cover.out | tail -n 1'
```

## Standard Packaging Commands

Use the repo scripts. They already enforce the Linux-only packaging flow.

### One artifact

```bash
sh ./scripts/package-linux-docker.sh \
  --version "$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
  --build-time "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" \
  --goos linux \
  --goarch amd64 \
  --output-dir /work/build/linux-go1.26-packages
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

- Do not build the release tarball on macOS.
- Do not generate checksums on macOS for release artifacts.
- Do not trust a host-side smoke test for a Linux binary.
- Do not mix ad-hoc packaging commands with the standard scripts unless you are
  actively fixing the packaging scripts themselves.

## If Something Fails

Check these first:

- `docker version`
- enough disk space for the container image and Go cache
- the repo is mounted at `/work`
- `GOCACHE` is set inside the container
- the target files do not already exist in `build/linux-go1.26-packages`

If packaging still fails:

- fix the repo scripts
- rerun the same standard commands
- do not switch to manual macOS packaging as a workaround
