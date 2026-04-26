# Requirements

`duplicacy-backup` is a Synology DSM operations tool. It is not a generic Linux
backup wrapper.

## Production Platform

Production use requires:

- Synology DSM
- Btrfs-backed `/volume*` storage
- a configured `source_path` that points at a Btrfs volume or subvolume root
  when the NAS is expected to run backups
- Duplicacy storage that is reachable from the operator profile used for the
  command

The application performs an early DSM platform check for operational commands.
If it does not detect DSM, it exits before running command handlers.

## Btrfs Is Required For Backups

Backups are snapshot-based by design. The configured `source_path` must be a
Btrfs volume or subvolume root so the tool can take a consistent read-only
snapshot before Duplicacy reads the data.

This is a correctness requirement. Backups intentionally refuse non-Btrfs source
paths rather than falling back to live-tree reads.

Restore-only disaster recovery access can omit `source_path` while the operator
is proving repository access on a replacement NAS. Once the replacement NAS is
expected to run backups, add `source_path` and validate that it is a Btrfs
volume or subvolume root.

## Non-Production Linux Use

Linux containers and Linux binaries are used for:

- automated tests
- coverage checks
- static analysis
- release packaging
- safe archive smoke checks such as `--version`, `--help`, and installer help

Those workflows validate the software artefacts, but they do not make a generic
Linux host a supported production runtime.
