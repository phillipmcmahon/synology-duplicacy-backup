# Update Trust Model

Use this reference when you need to understand what `duplicacy-backup update`
verifies before installing a managed release. For day-to-day update and
rollback commands, use [Operations](operations.md#upgrade-and-rollback).

The update command is designed for managed Synology installs that call the
stable command path at `/usr/local/bin/duplicacy-backup`.

At install time, `duplicacy-backup update`:

- reads release metadata from this project's GitHub Releases API
- selects the tarball for the current Linux platform
- accepts release asset URLs only from the expected GitHub release path, with
  redirects limited to GitHub-owned asset delivery hosts
- downloads the tarball and matching `.sha256` file over HTTPS
- verifies the tarball checksum before extraction
- optionally verifies the release-asset attestation before extraction
- extracts the package with path and symlink safety checks
- runs the packaged installer from the extracted package directory

By default, update uses `--attestations off` so existing scheduled NAS update
jobs keep checksum-only behaviour. Operators who want stronger verification can
opt in:

```bash
sudo /usr/local/bin/duplicacy-backup update --attestations required --yes
```

`--attestations required` needs GitHub CLI on `PATH` and stops before
extraction/install if release-asset attestation verification is unavailable or
fails. `--attestations auto` verifies when `gh` is available, skips
attestation verification when `gh` is missing, and still stops when
verification fails.

On unattended systems, `gh` must also be logged in for attestation verification;
otherwise use `--attestations auto` so scheduled updates can continue with
checksum verification when authenticated attestation checks are unavailable.

Attestation verification strengthens the normal update path, but it still
trusts GitHub as the release and attestation authority. If your threat model
includes a compromised GitHub release, compromised maintainer account, or
compromised GitHub attestation service, perform an out-of-band review before
installing.

For release publishing and verification rules, see
[Release Playbook](release-playbook.md).
