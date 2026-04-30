# Release Mirror

This note documents the repository's current NAS mirror destination. It is
site-specific release infrastructure, not a general operator requirement.

The public release source of truth is GitHub. After a tag-triggered release
workflow publishes the GitHub release, the release finalization step mirrors
the published artefacts to the NAS so installed systems can update from the
same local release tree used operationally.

## Current Mirror Destination

Default mirror settings used by the release scripts:

```text
host        : homestorage
remote root : /volume1/homes/phillipmcmahon/code/duplicacy-backup
latest      : /volume1/homes/phillipmcmahon/code/duplicacy-backup/latest/<tag>
archive     : /volume1/homes/phillipmcmahon/code/duplicacy-backup/archive/<tag>
```

The mirror contains:

- release tarballs
- per-asset `.sha256` files
- `SHA256SUMS.txt`
- GitHub-generated source archives

## Scripts

Use the release playbook first. These scripts are the implementation details
behind the finalization step:

```bash
sh ./scripts/finalize-release.sh --tag vX.Y.Z --issue <release-issue-number>
sh ./scripts/mirror-release-assets.sh --tag vX.Y.Z
sh ./scripts/verify-release.sh --tag vX.Y.Z
```

The scripts accept `--host` and `--remote-root` if the mirror location ever
changes. Keep this document in sync with those defaults when the site-specific
mirror changes.
