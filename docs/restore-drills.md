# Restore Drills

`duplicacy-backup` can now take a restore from planning through execution into
a safe drill workspace. It still does not copy data back to the live source.
That final copy-back remains an operator decision after inspection.

If you are connecting a replacement NAS to existing backup storage for the
first time, start with [new-nas-restore.md](new-nas-restore.md). This guide
assumes the tool is already installed and the selected repository can be read.

The safe pattern is simple:

- restore into a separate drill workspace
- inspect the restored data there
- copy only the intended files back to the live source path

Do not run a first restore directly over `/volume1/homes`, `/volume1/music`,
or any other live source path. A restore drill should prove that the backup can
be read without putting production data at risk.

## Workspace Ownership

Restore commands are designed to run as the operator user when that user can
read the selected config/secrets and write to the drill workspace. Create the
parent `--workspace-root` yourself first, preferably as a Synology shared
folder when DSM visibility matters. The tool preserves that parent folder and
creates the derived restore-job child underneath it with mode `0700`.

Avoid running restore through `sudo` just to work around workspace access.
Fix the workspace root ownership or permissions instead. Use `sudo` only for
manual folder creation, DSM shared-folder setup, or the later copy-back step
when the destination live path requires it.

## UX Model

There are two restore paths:

- `restore select` is the primary operator path. It is revision-first: choose
  a restore point, inspect it or restore from it, review the generated
  commands, then confirm the drill-workspace restore.
- `restore plan`, `restore list-revisions`, and `restore run` are
  the expert and scriptable primitives. Use them when you want step-by-step
  control, automation, or a runbook-friendly recovery procedure.

The picker sits on top of the explicit commands rather than replacing them.
That keeps emergency procedures copyable and avoids hidden live-data actions.

`restore list-revisions` uses a temporary workspace by default.
If you pass `--workspace`, it must already contain `.duplicacy/preferences`;
the listing commands will not create or rewrite persistent workspace
preferences.

`restore select` is a guided front end, not a second restore model. It refuses
non-interactive use, presents restore points, offers inspect-only or restore
actions, and uses a simple tree picker for selective restores. For restore
actions, it prints the exact `restore run` commands, asks for confirmation,
and then delegates to `restore run`. Multiple selected paths become multiple
explicit `restore run` commands into the same drill workspace. The picker is
convenience; the command model is the contract. It still never copies data
back to the live source.

Use `q` to cancel at restore-select text prompts or inside the tree picker.
Answering no at the final confirmation exits without restoring. `Ctrl-C`
remains an emergency interrupt if a process is already running; it is not the
normal exit path. If `Ctrl-C` is used during an active restore, the wrapper
cancels the running Duplicacy process, leaves the drill workspace in place,
does not delete partially restored data, and reports completed paths plus the
active path so the operator knows what remains to inspect or rerun.

## Start Here

Pick the label and target you want to prove. For most operator restores, start
with the guided flow:

```bash
duplicacy-backup config explain --target onsite-usb homes
sudo duplicacy-backup config validate --target onsite-usb homes
sudo duplicacy-backup health status --target onsite-usb homes
duplicacy-backup restore select --target onsite-usb homes
```

Use the expert primitives when you want a step-by-step runbook instead:

```bash
duplicacy-backup restore plan --target onsite-usb homes
duplicacy-backup restore list-revisions --target onsite-usb homes
duplicacy-backup restore run --target onsite-usb --revision <revision> --yes homes
```

`restore plan` shows the same `Storage` value as `config explain`, the
recommended drill workspace, and the Duplicacy commands that sit underneath
the wrapper. `restore run` creates or reuses the separate workspace and writes
`.duplicacy/preferences` there before restoring. For repositories created by
this tool, the Duplicacy snapshot ID is `data`.

If the selected storage backend needs credentials, `restore run` writes the
target keys from the label secrets file into the drill workspace preferences.
For a fully manual Duplicacy drill, use Duplicacy's normal password and key
handling. The same key names used in `[targets.<name>.keys]`, such as `s3_id`
and `s3_secret`, are the keys Duplicacy expects.

Primary Duplicacy references:

- [restore to a different folder or computer](https://github.com/gilbertchen/duplicacy/wiki/Restore-to-a-different-folder-or-computer)
- [restore command](https://github.com/gilbertchen/duplicacy/wiki/restore)
- [init command](https://github.com/gilbertchen/duplicacy/wiki/init)
- [managing passwords](https://github.com/gilbertchen/duplicacy/wiki/Managing-Passwords)

## Drill Workspace

For restore execution, the wrapper prepares the drill workspace for you. When
`--workspace` is omitted, it derives a predictable path from the restore job:

```text
/volume1/restore-drills/<label>-<target>-<restore-point-timestamp>-rev<id>
```

For example:

```text
/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev3
```

This default does not depend on `source_path`. `source_path` is only used as
live-source and copy-back context when it is configured.

Use `--workspace-root` when you want the tool to keep the predictable
restore-job folder name but place it under a root you choose. Create this root
yourself first, ideally as a Synology shared folder if you want DSM visibility:

```bash
sudo mkdir -p /volume1/restore-drills
```

```bash
duplicacy-backup restore run \
  --target onsite-usb \
  --revision 2403 \
  --workspace-root /volume1/restore-drills \
  --path 'phillipmcmahon/code/*' \
  --yes homes
```

That command derives a workspace such as:

```text
/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev2403
```

Use `--workspace` only when you want to provide the exact workspace path
yourself. `--workspace` and `--workspace-root` cannot be combined.

When you use `--workspace-root`, the tool preserves the existing root folder
permissions and creates or permissions only the derived restore-job child
folder. This lets `/volume1/restore-drills` remain a DSM-visible shared-folder
root while each job stays isolated below it. If you use `--workspace` and point
it directly at `/volume1/restore-drills`, that path is the workspace itself and
the tool may prepare or permission that exact folder.

Create a new, empty folder outside the live source tree only if you want to pin
the exact workspace name yourself with `--workspace`:

```bash
sudo mkdir -p /volume1/restore-drills/homes-onsite-usb
sudo chown "$(id -un):users" /volume1/restore-drills/homes-onsite-usb
cd /volume1/restore-drills/homes-onsite-usb
```

Restore execution creates the job workspace as the user running the command.
Keep restore inspection under that same user unless you deliberately change
ownership for a manual drill. The parent `--workspace-root` remains
operator-managed.

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

This prepares the drill workspace for manual Duplicacy use. If Duplicacy
reports that the storage is not initialised, stop and re-check the selected
target and storage value before continuing. `restore run` refuses to use the
live source path, refuses workspaces inside the live source tree, and refuses
non-empty unprepared workspaces.
During execution, `restore run` prints coloured status/progress to stderr,
including workspace preparation and the active restore operation. The final
restore report remains on stdout so it is still easy to save or paste into an
incident record. On success, the report keeps Duplicacy totals and suppresses
per-chunk and per-file download lines. On failure, it keeps Duplicacy
diagnostic lines when they are emitted; if Duplicacy fails without useful
detail, the report says so explicitly instead of showing unrelated tail output.
If an active restore is interrupted with `Ctrl-C`, the drill workspace is
retained. No automatic cleanup is performed because deleting partially restored
data could remove useful evidence during a recovery. Inspect the workspace, then
rerun the restore or remove the workspace manually when it is no longer useful.

## Choose A Restore Point

List available backup revisions with the wrapper:

```bash
duplicacy-backup restore list-revisions --target onsite-usb homes
```

Choose a revision that matches the recovery point you want to prove. Health
status also shows the latest known revision and age, which is useful for
confirming that the repository is current before a drill.

If you prefer the guided operator flow, use the picker:

```bash
duplicacy-backup restore select --target onsite-usb homes
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
- use `q` to cancel selection or quit inspection

The text prompts before the picker also accept `q` to cancel cleanly before
workspace preparation or restore execution. The guided flow prints progress
while loading restore points and revision contents. Once you confirm a restore
action, restore progress is printed to stderr while the final restore report
remains on stdout.

By default, restore actions use a workspace named from the selected restore
point, for example
`/volume1/restore-drills/homes-onsite-usb-20260424-070000-rev3`. That gives
the drill workspace an obvious link back to the restore point the operator
chose. Pass `--workspace-root` when you want that derived folder under a
specific shared-folder root. Pass `--workspace` explicitly only when you want
to pin the flow to one exact drill directory.

The common restore shapes are:

- inspect only: choose `Inspect revision contents only`, browse, then press `q`
- full restore: choose `Restore the full revision into the drill workspace`
- one file: move to the file, toggle it with `Space`, then press `g`
- one directory subtree: move to the directory, toggle it with `Space`, then press `g`; the picker generates a pattern such as `phillipmcmahon/code/*`
- several files or subtrees: keep moving and toggling items, then press `g`
- known subtree: start from `--path-prefix <path>` when you already know a useful branch

For large repositories, start from a useful subtree:

```bash
duplicacy-backup restore select --target onsite-usb --path-prefix phillipmcmahon/code homes
```

## NAS Permission Smoke Check

If `/volume1/restore-drills` is a Synology shared folder, validate the
shared-folder root before and after a small restore drill:

```bash
stat -c '%a %U:%G %n' /volume1/restore-drills
duplicacy-backup restore run \
  --target onsite-usb \
  --revision <revision> \
  --workspace-root /volume1/restore-drills \
  --path "relative/file/or/pattern" \
  --yes \
  homes
stat -c '%a %U:%G %n' /volume1/restore-drills
find /volume1/restore-drills -maxdepth 1 -type d -name '<label>-<target>-*' -print
# Example for homes/onsite-usb:
find /volume1/restore-drills -maxdepth 1 -type d -name 'homes-onsite-usb-*' -print
```

The two `stat` lines for `/volume1/restore-drills` should match. The derived
job folder beneath it is expected to be prepared for restore use and may have
stricter permissions owned by the user that ran the restore.

## Full Restore Drill

Run the restore into the drill workspace:

```bash
duplicacy-backup restore run \
  --target onsite-usb \
  --revision <revision> \
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
duplicacy-backup restore run \
  --target onsite-usb \
  --revision <revision> \
  --path "relative/file/or/pattern" \
  --yes \
  homes
```

Examples:

```bash
duplicacy-backup restore run --target onsite-usb --revision 2403 --path "phillipmcmahon/Documents/tax.pdf" --yes homes
duplicacy-backup restore run --target onsite-usb --revision 2403 --path "phillipmcmahon/Music/*" --yes homes
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
  /volume1/restore-drills/homes-onsite-usb-20260424-070000-rev3/phillipmcmahon/Documents/tax.pdf \
  /volume1/homes/phillipmcmahon/Documents/tax.pdf
```

If the dry run shows exactly what you expect, repeat without `--dry-run`:

```bash
sudo rsync -a \
  /volume1/restore-drills/homes-onsite-usb-20260424-070000-rev3/phillipmcmahon/Documents/tax.pdf \
  /volume1/homes/phillipmcmahon/Documents/tax.pdf
```

For directory restores, be careful with trailing slashes and always dry-run
first. After copying back, validate application-level behaviour rather than
assuming a file-level copy is sufficient.

## Drill Checklist

- Confirm the label and target with `config explain`.
- On an existing NAS, confirm `config validate` and `health status` before
  restoring.
- On a replacement NAS where `source_path` is not configured yet, confirm
  repository access with `restore list-revisions` before restoring.
- Use `restore run` for wrapper-managed drill execution. Use Duplicacy `init`
  only when you are deliberately running a fully manual Duplicacy drill.
- Use snapshot ID `data` for repositories created by this tool.
- Use `restore list-revisions` to choose a revision.
- Restore only into the drill workspace.
- Inspect restored data before copying anything back.
- Use `rsync --dry-run` before any live copy-back step.
- Record the label, target, revision, restored paths, and outcome.

## Current Boundary

`duplicacy-backup restore run` prepares or reuses a drill workspace and
executes `duplicacy restore` only inside that workspace. It refuses to use the
live source path, refuses source-child workspaces, and does not copy restored
data back. That keeps the wrapper useful for actual restore drills without
turning it into an unreviewed live-data mutation tool.

`duplicacy-backup restore select` is intentionally one step more conservative:
it keeps inspect-only read-only, and for restore actions it still shows the
explicit restore commands and requires confirmation before `restore run`
prepares the drill workspace. Any future command that automates
copy-back into live data should be designed as a separate capability with its
own safety review.
