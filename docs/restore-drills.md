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
- `restore run` restores a full revision, one file, or one directory pattern
  into the prepared workspace only.
- `restore select` interactively helps choose a revision and build a selection
  basket of files, directory subtrees, and manual patterns. It prints the
  explicit primitive commands, and can optionally execute them through
  `restore run` after confirmation.

The picker sits on top of the explicit commands rather than replacing them.
That keeps emergency procedures copyable and avoids hidden live-data actions.

`restore revisions` and `restore files` use a temporary workspace by default.
If you pass `--workspace`, it must already have been prepared with
`restore prepare`; the listing commands will not create or rewrite persistent
workspace preferences.

`restore select` is a guided front end, not a second restore model. It refuses
non-interactive use, guides revision and path selection through a simple
directory/file browser, then prints the exact `restore prepare` and
`restore run` commands. Multiple selected paths become multiple explicit
`restore run` commands into the same drill workspace. Without `--execute`, it
stops there. The picker is convenience; the command model is the contract.

If you add `--execute`, the picker still shows the generated `restore run`
command and asks for confirmation before delegating to `restore run`. This mode
requires the restore workspace to already be prepared. It still never copies
data back to the live source.

## What You Need

Pick the label and target you want to prove, then collect the current storage
details:

```bash
sudo duplicacy-backup config explain --target onsite-usb homes
sudo duplicacy-backup config validate --target onsite-usb homes
sudo duplicacy-backup health status --target onsite-usb homes
sudo duplicacy-backup restore plan --target onsite-usb homes
sudo duplicacy-backup restore prepare --target onsite-usb homes
sudo duplicacy-backup restore revisions --target onsite-usb homes
sudo duplicacy-backup restore select --target onsite-usb homes
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
sudo mkdir -p /volume1/restore-drills/homes-onsite-usb
sudo chown "$(id -un):users" /volume1/restore-drills/homes-onsite-usb
cd /volume1/restore-drills/homes-onsite-usb
```

The wrapper can prepare this workspace for you:

```bash
sudo duplicacy-backup restore prepare --target onsite-usb homes
cd /volume1/restore-drills/homes-onsite-usb
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

If you prefer a guided selection flow, use the picker:

```bash
sudo duplicacy-backup restore select --target onsite-usb homes
```

The picker prints the exact primitive command or commands to run. It does not
run `duplicacy restore` itself unless you pass `--execute` and confirm. In the
browser, build a selection basket:

- enter `open <number>` or a plain number to open a directory
- enter `add <number>` to add a displayed file or directory subtree
- enter `add <number>,<number>` to add several displayed items at once
- enter `add <path-or-pattern>` to type a snapshot-relative path or Duplicacy pattern manually
- enter `search <text>` to filter the current listing
- enter `clear` to remove the current search
- enter `selected` to review the basket
- enter `remove <number>` to remove an item from the basket
- enter `..` to move up one level
- enter `done` when the basket contains the files and subtrees you want
- enter `q` to quit

The common restore shapes are:

- full revision: answer `no` when asked whether to restore a specific path
- one file: browse to the file, then `add <number>` and `done`
- one directory subtree: browse to the directory, then `add <number>` and `done`; the picker generates a pattern such as `phillipmcmahon/code/*`
- several files or subtrees: keep browsing and adding items to the basket, then choose `done`
- known path or pattern: use `add phillipmcmahon/code/*` or another snapshot-relative value directly

For large repositories, start from a useful subtree:

```bash
sudo duplicacy-backup restore select --target onsite-usb --path-prefix phillipmcmahon/code homes
```

If the workspace is already prepared and you want the picker to execute the
selected restore after confirmation:

```bash
sudo duplicacy-backup restore select --target onsite-usb --execute homes
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
browse to the parent and enter `add <number>` for the directory you want, or
type the subtree pattern manually with `add phillipmcmahon/code/*`.

For several files or subtrees, add each one to the picker basket before
choosing `done`. The generated output remains explicit: one `restore run`
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
without `--execute`, it performs interactive selection and command generation
only. With `--execute`, it delegates to `restore run` after explicit
confirmation. Any future command that automates copy-back into live data should
be designed as a separate capability with its own safety review.
