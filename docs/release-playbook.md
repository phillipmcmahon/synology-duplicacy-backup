# Release Playbook

Use this checklist for every public release. Do not skip steps, do not
improvise the release notes from memory, and do not generate release artefacts
on the macOS host.

## Rules

- Release from a clean `main` tree only.
- Validate from the actual release tree, not from an older commit.
- Run release validation in Linux Go 1.26 only.
- Refresh coverage numbers before writing release notes.
- Public release notes must include `Highlights`, `Validation`, and `Coverage`.
- If one or more release attempts were superseded, fold their user-facing
  changes into the successful release notes so nothing important disappears.
- Let the tag-triggered GitHub Actions workflow build and publish the release
  artefacts.
- Do not build local release tarballs as part of the normal release flow.

## Checklist

### 1. Confirm scope

- Decide exactly which user-visible changes are shipping.
- Review `CHANGELOG.md` and fold superseded release-attempt notes into the next
  real release entry if needed.
- Update repo docs that describe the shipped behaviour:
  - `README.md`
  - `docs/cli.md`
  - `docs/operations.md`
  - `docs/configuration.md`
  - `docs/cheatsheet.md`
  - `TESTING.md`

### 2. Prepare version

- Update the version constant in `cmd/duplicacy-backup/main.go`.
- Add the new release entry to `CHANGELOG.md`.
- Make sure the changelog text reflects the release that will actually publish.

### 3. Validate the release tree in Linux

Use the standard Linux environment described in
[`linux-environment.md`](linux-environment.md).

Default command:

```bash
make release-prep
```

This command is the standard release-prep automation. It:

- checks that the tree is clean
- checks that you are on `main`
- runs Linux Go 1.26 validation
- captures coverage
- writes a draft release-notes file under `build/release-prep/`

Run these from the release candidate tree:

```bash
docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test ./...'

docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go vet ./...'

docker run --rm -v "$PWD":/work -w /work golang:1.26 /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test -cover ./...'
```

Capture:

- overall coverage
- `internal/workflow` coverage
- any package-specific coverage worth calling out in the release notes

If these numbers changed materially, update `TESTING.md`.

Local packaging is optional here and should be treated as a test-package flow
only, not as the source of truth for public release artefacts.

### 4. Write release notes

The public GitHub release body must use this format:

```text
## Highlights
- ...

## Validation
- Linux Go 1.26: go test ./...
- Linux Go 1.26: go vet ./...
- Linux Go 1.26: go test -cover ./...

## Coverage
- Linux Go 1.26: overall coverage = ...
- Linux Go 1.26: internal/workflow coverage = ...
```

Rules for release notes:

- Write for operators, not for developers reading git history.
- Include the important shipped story from any failed or superseded release
  attempts.
- Do not publish thin auto-generated notes if a richer hand-written summary is
  needed.

### 5. Commit and tag

- Commit the release prep changes.
- Push `main`.
- Tag the release from that exact commit.
- Push the tag.
- Let the tag-triggered GitHub Actions workflow publish the release artefacts.
- Do not manually upload local release tarballs to GitHub after tagging unless
  you are explicitly repairing a broken release.

Example:

```bash
git push origin main
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

### 6. Check the published release

After the release workflow finishes:

- confirm the GitHub release exists
- confirm the release notes body is correct
- confirm there is one canonical asset set only, with filenames like
  `duplicacy-backup_3.1.0_linux_amd64.tar.gz` and no duplicate `v3.1.0`
  variants
- confirm the artefacts were built from the tagged release commit
- if needed, edit the GitHub release body so it matches the validated release
  story

## Release Failure Rule

If a release workflow fails after the tag is pushed:

- fix `main` first
- do not pretend the failed tag is the real release
- cut a new patch release from the fixed tree
- fold the earlier release attempt notes into the successful release

Current example:

- `v2.1.4` and `v2.1.5` were superseded
- `v2.1.6` became the first successful public release carrying that combined
  feature set
