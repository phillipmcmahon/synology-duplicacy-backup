# Restore Drills

`duplicacy-backup` can now take a restore from planning through execution into
a safe drill workspace. It still does not copy data back to the live source.
That final copy-back remains an operator decision after inspection.

The safe pattern is simple:

- restore into a separate drill workspace
- inspect the restored data there
- copy only the intended files back to the live source path

Do not run a first restore directly over `/volume1/homes`, `/volume1/music`,
or any other live source path. A restore drill should prove that the backup can
be read without putting production data at risk.

## UX Model

Restore commands follow a flag-first model so they are safe for scheduled,
scripted, and documented recovery procedures:

- `restore plan` explains the selected label and target.
- `restore prepare` creates the safe workspace.
- `restore revisions` lists recovery points.
- `restore files` inspects a selected revision.
- `restore run` restores a full revision or one snapshot-relative path into
  the prepared workspace only.
- `restore select` interactively helps choose a revision/path and prints the
  explicit primitive commands to run.

An interactive picker can still be added later, but it should sit on top of
these explicit commands rather than replacing them. That keeps emergency
procedures copyable and avoids hidden live-data actions.

`restore revisions` and `restore files` use a temporary workspace by default.
If you pass `--workspace`, it must already have been prepared with
`restore prepare`; the listing commands will not create or rewrite persistent
workspace preferences.

`restore select` is a command generator, not a second restore model. It refuses
non-interactive use, guides revision and path selection, then prints the exact
`restore prepare` and `restore run` commands to execute explicitly. The picker
is convenience; the command model is the contract.

## What You Need

Pick the label and target you want to prove, then collect the current storage
details:

```bash
sudo duplicacy-backup config explain --target onsite-usb2 homes
sudo duplicacy-backup config validate --target onsite-usb2 homes
sudo duplicacy-backup health status --target onsite-usb2 homes
sudo duplicacy-backup restore plan --target onsite-usb2 homes
sudo duplicacy-backup restore prepare --target onsite-usb2 homes
sudo duplicacy-backup restore revisions --target onsite-usb2 homes
sudo duplicacy-backup restore select --target onsite-usb2 homes
```

Use the `Storage` value from `config explain` as the Duplicacy storage URL for
the drill. `restore plan` prints that same value, the recommended drill
workspace, and the Duplicacy commands that sit underneath the wrapper.
`restore prepare` creates that separate workspace and writes
`.duplicacy/preferences` there, but does not run a restore. For repositories
created by this tool, the Duplicacy snapshot ID is `data`.

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

The wrapper can prepare this workspace for you:

```bash
sudo duplicacy-backup restore prepare --target onsite-usb2 homes
cd /volume1/restore-drills/homes-onsite-usb2
```

When run with `sudo`, `restore prepare` creates the drill workspace as a
root-owned workspace by design. Keep restore inspection and Duplicacy commands
under `sudo` unless you deliberately change ownership for a manual drill.

If you are preparing the workspace manually instead, initialise this folder as
a temporary Duplicacy repository that points at the same storage used by the
backup target:

```bash
duplicacy init data "/volumeUSB2/usbshare/duplicacy/homes"
```

For S3-compatible storage, use the full `storage` value from the label config:

```bash
duplicacy init data "s3://EU@gateway.storjshare.io/bucket-id/homes"
```

This prepares the drill workspace. If Duplicacy reports that the storage is not
initialised, stop and re-check the selected target and storage value before
continuing. `restore prepare` refuses to use the live source path, refuses
workspaces inside the live source tree, and refuses non-empty workspaces except
for an existing `.duplicacy` directory from an earlier preparation.

## Choose A Revision

List available revisions with the wrapper:

```bash
sudo duplicacy-backup restore revisions --target onsite-usb2 homes
```

To inspect file names in a specific revision:

```bash
sudo duplicacy-backup restore files --target onsite-usb2 --revision <revision> homes
sudo duplicacy-backup restore files --target onsite-usb2 --revision <revision> --path "relative/path" homes
```

Choose a revision that matches the recovery point you want to prove. Health
status also shows the latest known revision and age, which is useful for
confirming that the repository is current before a drill.

If you prefer a guided selection flow, use the picker:

```bash
sudo duplicacy-backup restore select --target onsite-usb2 homes
```

The picker prints the exact primitive command to run. It does not run
`duplicacy restore` itself.

## Full Restore Drill

Run the restore into the prepared drill workspace:

```bash
sudo duplicacy-backup restore run \
  --target onsite-usb2 \
  --revision <revision> \
  --workspace /volume1/restore-drills/homes-onsite-usb2 \
  --yes \
  homes
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
sudo duplicacy-backup restore run \
  --target onsite-usb2 \
  --revision <revision> \
  --path "relative/path/from/snapshot" \
  --workspace /volume1/restore-drills/homes-onsite-usb2 \
  --yes \
  homes
```

Examples:

```bash
sudo duplicacy-backup restore run --target onsite-usb2 --revision 2403 --path "phillipmcmahon/Documents/tax.pdf" --workspace /volume1/restore-drills/homes-onsite-usb2 --yes homes
sudo duplicacy-backup restore run --target onsite-usb2 --revision 2403 --path "phillipmcmahon/Music/" --workspace /volume1/restore-drills/homes-onsite-usb2 --yes homes
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
- Use `restore prepare` or Duplicacy `init` to prepare a separate drill
  workspace, never directly over live data.
- Use snapshot ID `data` for repositories created by this tool.
- Use `restore revisions` to choose a revision.
- Use `restore files --revision <id>` before selective restores.
- Restore only into the drill workspace.
- Inspect restored data before copying anything back.
- Use `rsync --dry-run` before any live copy-back step.
- Record the label, target, revision, restored paths, and outcome.

## Current Boundary

`duplicacy-backup restore run` executes `duplicacy restore` only inside a
prepared drill workspace. It refuses to use the live source path, refuses
source-child workspaces, and does not copy restored data back. That keeps the
wrapper useful for actual restore drills without turning it into an
unreviewed live-data mutation tool.

`duplicacy-backup restore select` is intentionally one step more conservative:
it performs interactive selection and command generation only. It does not
restore data. Any future command that automates copy-back into live data should
be designed as a separate capability with its own safety review.
