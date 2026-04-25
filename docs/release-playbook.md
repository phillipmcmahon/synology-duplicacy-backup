# Release Playbook

Use this checklist for every public release. Do not skip steps, improvise the
release notes from memory, or generate release artefacts on the macOS host.

## Rules

- Release from a clean `main` tree only.
- Validate from the actual release tree, not from an older commit.
- Run release validation in Linux Go 1.26 only.
- Run Staticcheck as part of release validation; CI uses
  `honnef.co/go/tools/cmd/staticcheck` and the version is pinned in Go module
  metadata so Dependabot can update it.
- Use the Go container image declared in
  [`tools/release-validation/Dockerfile`](../tools/release-validation/Dockerfile);
  Dependabot monitors that Dockerfile for image updates.
- Refresh coverage numbers before writing release notes.
- Public release notes must include `Highlights`, `Validation`, and `Coverage`.
- Keep `CHANGELOG.md` as the repo-rooted, offline-greppable mirror of the
  operator-facing GitHub release story.
- If one or more release attempts were superseded, fold their user-facing
  changes into the successful release notes so nothing important disappears.
- Let the tag-triggered GitHub Actions workflow build and publish the release
  artefacts.
- Keep GitHub immutable releases enabled for the repository. The release
  workflow depends on that setting for the release attestations verified by
  `gh release verify` and `gh release verify-asset`.
- Do not build local release tarballs as part of the normal release flow.
- After the GitHub release is live, download the published release artefacts
  plus the GitHub-generated source archives and mirror them to
  `homestorage:/volume1/homes/phillipmcmahon/code/duplicacy-backup/latest/<tag>/`.
  Older release directories are kept under
  `homestorage:/volume1/homes/phillipmcmahon/code/duplicacy-backup/archive/<tag>/`.

## Release Tracking Conventions

Use the project board and release issues in a lightweight, repeatable way:

- Create one release-prep issue for each actual release.
- Use focused child tasks under `#24` for release and operational-tooling
  improvements such as mirroring, verification, or baseline reconciliation.
- Move each active release item through `Ready` -> `In Progress` -> `Done`.
- Close each release child with a short comment that includes:
  - landed commit
  - validation run
  - published release URL
  - NAS mirror result
  - full release verification result
- Do not create a separate publish issue by default. Treat tag-and-publish as
  the operational completion of the prepared release unless a failed release or
  repair path is substantial enough to justify its own issue.

Recommended project support:

- Keep `#24` as the umbrella for release and operational-tooling follow-ons.
- Optionally create a saved project view in the GitHub UI filtered to
  `area:release` items so release work is easier to review at a glance.

## Checklist

### 1. Confirm Scope

- Decide exactly which user-visible changes are shipping.
- Review `CHANGELOG.md` and fold superseded release-attempt notes into the next
  real release entry if needed.
- Create or update the release-prep issue and move it to `In Progress`.
- Update repo docs that describe the shipped behaviour:
  - `README.md`
  - `docs/cli.md`
  - `docs/operations.md`
  - `docs/configuration.md`
  - `docs/cheatsheet.md`
  - `TESTING.md`

Suggested release-prep checklist:

- [ ] version metadata updated
- [ ] changelog entry added or refreshed
- [ ] testing baseline refreshed
- [ ] Linux Go 1.26 validation passed
- [ ] release-prep notes generated
- [ ] prep commit pushed to `main`
- [ ] release tag pushed from the validated commit
- [ ] GitHub release workflow passed
- [ ] release finalized with `scripts/finalize-release.sh`
- [ ] closure summary pasted into the release issue

### 2. Prepare Version

- Leave the default version fallback in `cmd/duplicacy-backup/main.go` as
  `dev`; release and package builds inject the real version with `-ldflags`.
- Add the new release entry to `CHANGELOG.md`.
- Make sure the changelog text reflects the release that will actually publish.

### 3. Validate the Release Tree in Linux

Use the standard Linux environment described in
[`linux-environment.md`](linux-environment.md).

Default command:

```bash
make release-prep RELEASE_VERSION=4.x.y
```

This command is the standard release-prep automation. It:

- checks that the tree is clean
- checks that you are on `main`
- checks that the source fallback version is still `dev`
- checks that `CHANGELOG.md` contains the requested release entry
- runs Linux Go 1.26 validation
- captures coverage
- writes a draft release-notes file under `build/release-prep/`

Run these from the release candidate tree:

```bash
GO_CONTAINER_IMAGE="$(awk 'toupper($1) == "FROM" { print $2; exit }' tools/release-validation/Dockerfile)"

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test ./...'

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go vet ./...'

docker run --rm -v "$PWD":/work -w /work "$GO_CONTAINER_IMAGE" /bin/sh -lc \
  'export GOCACHE=/tmp/gocache && /usr/local/go/bin/go test -cover ./...'
```

Capture:

- overall coverage
- `internal/workflow` coverage
- any package-specific coverage worth calling out in the release notes

If these numbers changed materially, update `TESTING.md`.

Local packaging is optional here and should be treated as a test-package flow
only, not as the source of truth for public release artefacts. Any local test
package must be written under the structured `build/test-packages` tree:

- `build/test-packages/release/<run-id>/` for standard `duplicacy-backup`
  package output and NAS smoke bundles
- `build/test-packages/poc/<name>/<run-id>/` for experimental or
  proof-of-concept bundles

Do not create ad-hoc package directories elsewhere under `build/`, and do not
drop new artefacts flat into `build/test-packages` or
`build/test-packages/release`.

Use `scripts/package-test-bundle.sh` for operator-facing smoke packages that
need bundled instructions. It creates one self-contained per-run folder and a
bundle whose extracted layout is `README.md`, `artifacts/`, `checksums/`, and
`instructions/`.

### 4. Write Release Notes

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

### 5. Commit and Tag

- Commit the release prep changes.
- Push `main`.
- Tag the release from that exact commit, using the validated release notes as
  the annotated tag message.
- Push the tag.
- Let the tag-triggered GitHub Actions workflow publish the release artefacts.
- Do not manually upload local release tarballs to GitHub after tagging unless
  you are explicitly repairing a broken release.
- Keep `build/release-prep/` outputs local. They are generated release working
  notes for tagging and validation, not source artefacts to commit.

Example:

```bash
git push origin main
git tag -a vX.Y.Z --cleanup=verbatim -F build/release-prep/vX.Y.Z/release-notes.md
git push origin vX.Y.Z
```

### 6. Check the Published Release

After the release workflow finishes:

- confirm the GitHub release exists
- confirm the release notes body is correct
- confirm there is one canonical asset set only, with filenames like
  `duplicacy-backup_3.1.0_linux_amd64.tar.gz` and no duplicate `v3.1.0`
  variants
- confirm the GitHub release and each release asset verify against GitHub
  release attestations
- confirm the artefacts were built from the tagged release commit
- if needed, edit the GitHub release body so it matches the validated release
  story
- do not close the release-prep issue yet; full verification runs after the NAS
  mirror step

For a historical release published before release attestations were enabled,
use `--skip-attestations`. Do not use that option for new releases.

To manually verify the release attestation:

```bash
gh release verify vX.Y.Z --repo phillipmcmahon/synology-duplicacy-backup
```

To manually verify one downloaded asset:

```bash
gh release verify-asset vX.Y.Z ./duplicacy-backup_X.Y.Z_linux_amd64.tar.gz \
  --repo phillipmcmahon/synology-duplicacy-backup
```

### 7. Finalize the Release

After the release exists and the GitHub Actions asset set is complete:

- download all published release assets from GitHub
- download the GitHub-generated source archives:
  - `Source code (zip)`
  - `Source code (tar.gz)`
- create the destination directory:
  - `/volume1/homes/phillipmcmahon/code/duplicacy-backup/latest/<tag>/`
- move older release mirror directories under:
  - `/volume1/homes/phillipmcmahon/code/duplicacy-backup/archive/<tag>/`
- mirror the full artefact set to homestorage
- run the full release verifier
- paste the generated closure summary into the release issue before closing it

Supported command:

```bash
sh ./scripts/finalize-release.sh --tag vX.Y.Z --issue <release-issue-number>
```

This is the standard release closure gate. It runs
`scripts/mirror-release-assets.sh`, then `scripts/verify-release.sh`, then
prints a concise release-issue comment that includes the GitHub release URL,
NAS mirror path, verification result, and attestation result.

Expected artefacts for each release:

- `duplicacy-backup_<version>_linux_amd64.tar.gz`
- `duplicacy-backup_<version>_linux_amd64.tar.gz.sha256`
- `duplicacy-backup_<version>_linux_arm64.tar.gz`
- `duplicacy-backup_<version>_linux_arm64.tar.gz.sha256`
- `duplicacy-backup_<version>_linux_armv7.tar.gz`
- `duplicacy-backup_<version>_linux_armv7.tar.gz.sha256`
- `SHA256SUMS.txt`
- `Source code (zip)`
- `Source code (tar.gz)`

If you need to repair only the mirror step, use:

```bash
sh ./scripts/mirror-release-assets.sh --tag vX.Y.Z
```

The script downloads the published release assets plus the two GitHub source
archives into a local staging directory, creates `latest/` and `archive/` if
needed, archives older release directories, creates the remote latest release
directory, and mirrors the files with a `tar`-over-SSH transfer
(`tar -cf - . | ssh ...`).
This avoids the filename
and wildcard edge cases we saw from plain `scp` when copying files such as
`Source code (zip)` to Synology.

### 8. Manually Verify the Complete Release

`scripts/finalize-release.sh` runs the full verifier automatically. If you need
to rerun verification without re-mirroring, use:

```bash
sh ./scripts/verify-release.sh --tag vX.Y.Z
```

The script verifies the published GitHub release, required release-note
headings, expected packaged assets, GitHub release attestations, individual
asset attestations, local-versus-remote tag commit alignment, and the mirrored
artefact set under `homestorage` `latest/<tag>`.

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
