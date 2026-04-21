# Restore Drills

`duplicacy-backup` does not run restores today. Restore drills use the
Duplicacy CLI directly, with `duplicacy-backup` used first to confirm the
label, target, storage value, and repository health.

The safe pattern is simple:

- restore into a separate drill workspace
- inspect the restored data there
- copy only the intended files back to the live source path

Do not run a first restore directly over `/volume1/homes`, `/volume1/music`,
or any other live source path. A restore drill should prove that the backup can
be read without putting production data at risk.

## What You Need

Pick the label and target you want to prove, then collect the current storage
details:

```bash
sudo duplicacy-backup config explain --target onsite-usb2 homes
sudo duplicacy-backup config validate --target onsite-usb2 homes
sudo duplicacy-backup health status --target onsite-usb2 homes
```

Use the `Storage` value from `config explain` as the Duplicacy storage URL for
the drill. For repositories created by this tool, the Duplicacy snapshot ID is
`data`.

If the selected storage backend needs credentials, configure the temporary
Duplicacy workspace using Duplicacy's normal password and key handling. The
same key names used in `[targets.<name>.keys]`, such as `s3_id` and
`s3_secret`, are the keys Duplicacy expects.

Primary Duplicacy references:

- [restore to a different folder or computer](https://github.com/gilbertchen/duplicacy/wiki/Restore-to-a-different-folder-or-computer)
- [restore command](https://github.com/gilbertchen/duplicacy/wiki/restore)
- [init command](https://github.com/gilbertchen/duplicacy/wiki/init)
- [managing passwords](https://github.com/gilbertchen/duplicacy/wiki/Managing-Passwords)

## Prepare A Drill Workspace

Create a new, empty folder outside the live source tree:

```bash
sudo mkdir -p /volume1/restore-drills/homes-onsite-usb2
sudo chown "$(id -un):users" /volume1/restore-drills/homes-onsite-usb2
cd /volume1/restore-drills/homes-onsite-usb2
```

Initialise this folder as a temporary Duplicacy repository that points at the
same storage used by the backup target:

```bash
duplicacy init data "/volumeUSB2/usbshare/duplicacy/homes"
```

For S3-compatible storage, use the full `storage` value from the label config:

```bash
duplicacy init data "s3://EU@gateway.storjshare.io/bucket-id/homes"
```

This prepares the drill workspace. If Duplicacy reports that the storage is not
initialised, stop and re-check the selected target and storage value before
continuing.

## Choose A Revision

List available revisions from the drill workspace:

```bash
duplicacy list
```

To inspect file names in a specific revision:

```bash
duplicacy list -files -r <revision>
```

Choose a revision that matches the recovery point you want to prove. Health
status also shows the latest known revision and age, which is useful for
confirming that the repository is current before a drill.

## Full Restore Drill

Run the restore inside the drill workspace:

```bash
duplicacy restore -r <revision> -stats
```

Because the drill workspace starts empty, avoid `-overwrite` and `-delete`.
Those flags are useful in some Duplicacy workflows, but they are not needed
for a safe first restore drill.

If you are restoring on a different machine or under a normal user account,
consider Duplicacy's `-ignore-owner` option for the drill. If the drill is
intended to prove Synology ownership restoration, run the restore in the
appropriate NAS context and omit `-ignore-owner`.

After the restore completes, inspect the restored tree:

```bash
find . -maxdepth 3 -type f | head
du -sh .
```

Open a few representative files, confirm ownership and modes where they
matter, and record the revision that was successfully restored.

## Selective Restore Drill

Duplicacy restore accepts command-line patterns, so you can restore only a
specific file or directory into the drill workspace:

```bash
duplicacy restore -r <revision> -stats -- "relative/path/from/snapshot"
```

Examples:

```bash
duplicacy restore -r 2403 -stats -- "phillipmcmahon/Documents/tax.pdf"
duplicacy restore -r 2403 -stats -- "phillipmcmahon/Music/"
```

Use paths relative to the snapshot root, not absolute NAS paths. For example,
use `phillipmcmahon/Documents/tax.pdf`, not
`/volume1/homes/phillipmcmahon/Documents/tax.pdf`.

## Copy Back Deliberately

Only copy data back after you have inspected the restored files and confirmed
the target location. Start with `rsync --dry-run`:

```bash
rsync -a --dry-run \
  /volume1/restore-drills/homes-onsite-usb2/phillipmcmahon/Documents/tax.pdf \
  /volume1/homes/phillipmcmahon/Documents/tax.pdf
```

If the dry run shows exactly what you expect, repeat without `--dry-run`:

```bash
sudo rsync -a \
  /volume1/restore-drills/homes-onsite-usb2/phillipmcmahon/Documents/tax.pdf \
  /volume1/homes/phillipmcmahon/Documents/tax.pdf
```

For directory restores, be careful with trailing slashes and always dry-run
first. After copying back, validate application-level behaviour rather than
assuming a file-level copy is sufficient.

## Drill Checklist

- Confirm the label and target with `config explain`.
- Confirm `config validate` and `health status` before restoring.
- Restore into a separate drill workspace, never directly over live data.
- Use snapshot ID `data` for repositories created by this tool.
- Use `duplicacy list` to choose a revision.
- Use `duplicacy list -files -r <revision>` before selective restores.
- Restore only into the drill workspace.
- Inspect restored data before copying anything back.
- Use `rsync --dry-run` before any live copy-back step.
- Record the label, target, revision, restored paths, and outcome.

## Current Boundary

This guide documents the safe manual process. A future
`duplicacy-backup restore plan` command should be read-only and should generate
the relevant Duplicacy commands from the selected label and target. It should
not perform a restore until the operator experience is designed carefully
enough to handle full and selective recovery without surprises.
