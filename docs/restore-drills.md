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

There are two restore paths:

- `restore select` is the primary operator path. It is revision-first: choose
  a restore point, inspect it or restore from it, review the generated
  commands, then confirm the drill-workspace restore.
- `restore plan`, `restore prepare`, `restore revisions`, `restore files`, and
  `restore run` are the expert and scriptable primitives. Use them when you
  want step-by-step control, automation, or a runbook-friendly recovery
  procedure.

The picker sits on top of the explicit commands rather than replacing them.
That keeps emergency procedures copyable and avoids hidden live-data actions.

`restore revisions` and `restore files` use a temporary workspace by default.
If you pass `--workspace`, it must already have been prepared with
`restore prepare`; the listing commands will not create or rewrite persistent
workspace preferences.

`restore select` is a guided front end, not a second restore model. It refuses
non-interactive use, presents restore points, offers inspect-only or restore
actions, and uses a simple tree picker for selective restores. For restore
actions, it prints the exact `restore prepare` and `restore run` commands,
asks for confirmation, prepares the drill workspace when needed, and then
delegates to `restore run`. Multiple selected paths become multiple explicit
`restore run` commands into the same drill workspace. The picker is
convenience; the command model is the contract. It still never copies data
back to the live source.

## Start Here

Pick the label and target you want to prove. For most operator restores, start
with the guided flow:

```bash
sudo duplicacy-backup config explain --target onsite-usb homes
sudo duplicacy-backup config validate --target onsite-usb homes
sudo duplicacy-backup health status --target onsite-usb homes
sudo duplicacy-backup restore select --target onsite-usb homes
```

Use the expert primitives when you want a step-by-step runbook instead:

```bash
sudo duplicacy-backup restore plan --target onsite-usb homes
sudo duplicacy-backup restore prepare --target onsite-usb homes
sudo duplicacy-backup restore revisions --target onsite-usb homes
sudo duplicacy-backup restore files --target onsite-usb --revision <revision> --path "relative/path" homes
sudo duplicacy-backup restore run --target onsite-usb --revision <revision> --workspace <workspace> --yes homes
```

`restore plan` shows the same `Storage` value as `config explain`, the
recommended drill workspace, and the Duplicacy commands that sit underneath
the wrapper. `restore prepare` creates that separate workspace and writes
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

Create a new, empty folder outside the live source tree if you want to pin the
workspace name yourself:

```bash
sudo mkdir -p /volume1/restore-drills/homes-onsite-usb
sudo chown "$(id -un):users" /volume1/restore-drills/homes-onsite-usb
cd /volume1/restore-drills/homes-onsite-usb
```

The wrapper can prepare this workspace for you:

```bash
sudo duplicacy-backup restore prepare --target onsite-usb --workspace /volume1/restore-drills/homes-onsite-usb homes
cd /volume1/restore-drills/homes-onsite-usb
```

If you omit `--workspace`, `restore prepare` creates a timestamped drill
workspace instead.

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
sudo duplicacy-backup restore revisions --target onsite-usb homes
```

To inspect file names in a specific revision:

```bash
sudo duplicacy-backup restore files --target onsite-usb --revision <revision> homes
sudo duplicacy-backup restore files --target onsite-usb --revision <revision> --path "relative/path" homes
```

Choose a revision that matches the recovery point you want to prove. Health
status also shows the latest known revision and age, which is useful for
confirming that the repository is current before a drill.

If you prefer the guided operator flow, use the picker:

```bash
sudo duplicacy-backup restore select --target onsite-usb homes
```

The picker prints the exact primitive command or commands that will be used for
restore actions. It still asks for confirmation before any restore is
performed. In the tree picker:

- use the arrow keys to move through the snapshot tree
- use `Right` to expand directories
- use `Left` to collapse directories or move back up
- use `Space` to select or clear the current file or subtree
- use `Tab` to switch between the tree and the primitive detail pane
- use `g` to continue with the current selection and generate the restore commands
- use `q` to quit

By default, `restore prepare` creates a timestamped drill workspace under
`restore-drills`, for example
`/volume1/restore-drills/homes-onsite-usb-20260424-081530`. In the guided
`restore select` flow, restore actions instead default to a workspace named
from the selected restore point, for example
`/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev3`. That gives
the drill workspace an obvious link back to the restore point the operator
chose. Pass `--workspace` explicitly whenever you want to pin the flow to one
known drill directory.

The common restore shapes are:

- inspect only: choose `Inspect revision contents only`, browse, then press `q`
- full restore: choose `Restore the full revision into the drill workspace`
- one file: move to the file, toggle it with `Space`, then press `g`
- one directory subtree: move to the directory, toggle it with `Space`, then press `g`; the picker generates a pattern such as `phillipmcmahon/code/*`
- several files or subtrees: keep moving and toggling items, then press `g`
- known subtree: start from `--path-prefix <path>` when you already know a useful branch

For large repositories, start from a useful subtree:

```bash
sudo duplicacy-backup restore select --target onsite-usb --path-prefix phillipmcmahon/code homes
```

## Full Restore Drill

Run the restore into the prepared drill workspace:

```bash
sudo duplicacy-backup restore run \
  --target onsite-usb \
  --revision <revision> \
  --workspace /volume1/restore-drills/homes-onsite-usb \
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

Duplicacy restore accepts command-line patterns, so you can restore one file
or a directory subtree into the drill workspace:

```bash
sudo duplicacy-backup restore run \
  --target onsite-usb \
  --revision <revision> \
  --path "relative/file/or/pattern" \
  --workspace /volume1/restore-drills/homes-onsite-usb \
  --yes \
  homes
```

Examples:

```bash
sudo duplicacy-backup restore run --target onsite-usb --revision 2403 --path "phillipmcmahon/Documents/tax.pdf" --workspace /volume1/restore-drills/homes-onsite-usb --yes homes
sudo duplicacy-backup restore run --target onsite-usb --revision 2403 --path "phillipmcmahon/Music/*" --workspace /volume1/restore-drills/homes-onsite-usb --yes homes
```

Use paths relative to the snapshot root, not absolute NAS paths. For example,
use `phillipmcmahon/Documents/tax.pdf`, not
`/volume1/homes/phillipmcmahon/Documents/tax.pdf`.

For a directory subtree, pass a Duplicacy pattern such as
`phillipmcmahon/code/*`. The picker can select a directory subtree for you:
move to the directory in the tree and toggle it with `Space`.

For several files or subtrees, keep toggling the items you want before
pressing `g`. The generated output remains explicit: one `restore run`
command is produced for each selected path or pattern.

## Copy Back Deliberately

Only copy data back after you have inspected the restored files and confirmed
the target location. Start with `rsync --dry-run`:

```bash
rsync -a --dry-run \
  /volume1/restore-drills/homes-onsite-usb/phillipmcmahon/Documents/tax.pdf \
  /volume1/homes/phillipmcmahon/Documents/tax.pdf
```

If the dry run shows exactly what you expect, repeat without `--dry-run`:

```bash
sudo rsync -a \
  /volume1/restore-drills/homes-onsite-usb/phillipmcmahon/Documents/tax.pdf \
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
it keeps inspect-only read-only, and for restore actions it still shows the
explicit restore commands and requires confirmation before preparing the drill
workspace and delegating to `restore run`. Any future command that automates
copy-back into live data should be designed as a separate capability with its
own safety review.
