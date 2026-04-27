# Troubleshooting

Use this guide when a scheduled task, health check, update, or notification
does something surprising and you need the next useful diagnostic step.

The entries are intentionally short. They explain the likely meaning, suggest
the safest next action, and link back to the source-of-truth docs rather than
repeating the full operating model here.

## First Checks

Start with the exact label and target from the failed output. Most commands are
target-scoped, so checking the wrong target can make a real problem look
inconsistent.

Useful first commands:

```bash
duplicacy-backup config validate --target <target> <label>
duplicacy-backup health status --target <target> <label>
```

If the problem came from Synology Task Scheduler, also read the task's captured
standard output/error. Scheduler status tells you whether the command exited
successfully; the command output tells you why.

## Scheduled Task Failed but Health Is Healthy

This usually means the scheduler saw a command failure, while `health status`
is reporting the current backup health state from the last known state file.
Those are related signals, but they are not the same signal.

Check the scheduled task command, the selected `--target`, and the captured
standard output/error from the failed run. If the task was a maintenance
command such as prune, cleanup, or update, it can fail even when the
latest backup state is still healthy.

See also:

- [Workflow and scheduling](workflow-scheduling.md)
- [Operator cheat sheet](cheatsheet.md#health-checks)
- [Operations](operations.md#synology-task-scheduler)

## Repository Access Is Not Initialized

`Repository Access : Not initialized` means the selected storage value was
reachable, but the repository does not yet exist there for that label and
target. This is different from `Invalid`, which means access is broken or the
storage cannot be read correctly.

Confirm the target name and full `storage` value in the label config. Do not
expect `config validate` to initialise storage; it is read-only. Once the
repository has been intentionally initialised, validation should move from `Not
initialized` to `Valid`.

See also:

- [Configuration and secrets](configuration.md#output-model)
- [Operator cheat sheet](cheatsheet.md#config-and-validation)

## Repository Access Requires Sudo

`Repository Access : Requires sudo` means the selected target uses path-based
local repository storage. Backups write local repository chunk and snapshot
metadata as root so ordinary users cannot inspect or mutate backup internals
outside the tool's policy boundary.

Run the same validation through `sudo` from the operator account:

```bash
sudo duplicacy-backup config validate --target <target> <label>
```

This is different from `Invalid`: the repository may be healthy, but the
readiness probe needs root access to inspect protected local metadata. Object
and remote repositories continue to validate as the operator user because their
authority boundary is the configured storage credentials.

See also:

- [Privilege model](privilege-model.md#repository-mutation-boundary)
- [Configuration and secrets](configuration.md#output-model)

## Health Verify Is Degraded or Unhealthy

`health verify` uses health-specific exit codes: `0` for healthy, `1` for
degraded, and `2` for unhealthy. An unhealthy result does not always mean a
new backup just failed; it can also mean integrity, recency, or repository
state no longer meets policy.

Run a JSON summary when you need precise automation detail:

```bash
duplicacy-backup health verify --json-summary --target <target> <label>
```

Look for `failure_code`, `failure_codes`, and `recommended_action_codes`, then
follow the recommendation before changing retention or pruning policy.

See also:

- [Configuration and secrets](configuration.md#health-state)
- [Operator cheat sheet](cheatsheet.md#health-checks)

## Storage Credentials Fail During Validation Or Backup

Storage keys are passed through to Duplicacy using the names under
`[targets.<name>.keys]` in the secrets file. The wrapper checks that required
keys are present for known backends such as S3-compatible storage, but it does
not enforce provider-specific credential lengths. If a credential value is the
wrong length, expired, or belongs to the wrong backend, use Duplicacy's own
error output as the source of truth.

For S3-compatible storage, including Storj gateway and Duplicacy's `s3c://`,
`minio://`, and `minios://` schemes, use generic Duplicacy key names:

```toml
[targets.offsite-storj.keys]
s3_id = "..."
s3_secret = "..."
```

For native Duplicacy `storj://` storage, use:

```toml
[targets.offsite-storj.keys]
storj_key = "..."
storj_passphrase = "..."
```

See also:

- [Configuration and secrets](configuration.md#secrets)
- [Duplicacy storage backends](https://github.com/gilbertchen/duplicacy/wiki/Storage-Backends)

## I Need To Restore Data

The wrapper restores only into a separate drill workspace. It does not copy
restored data back to the live source path.

If this is a replacement NAS and you have not connected it to the existing
backup repository yet, start with [Restore onto a new NAS](new-nas-restore.md).

For most operator restores, start with the guided flow:

```bash
duplicacy-backup restore select --target <target> <label>
```

That path is revision-first: choose a restore point, inspect it read-only or
restore from it, review the generated commands, then confirm the drill restore.

Use `restore plan` when you want the explicit expert path and safe next-step commands:

```bash
duplicacy-backup restore plan --target <target> <label>
```

If `restore select` fails before the picker appears, check that the command is
running in an interactive terminal. The tree picker needs a TTY and a usable
terminal type. When connecting over SSH, use `ssh -t ...`; avoid `TERM=dumb`.
If an interactive terminal is not available, use the scriptable primitives:
`restore list-revisions` to choose a revision and `restore run` with explicit
`--revision`, `--path`, and workspace options.

List revisions before restoring:

```bash
duplicacy-backup restore list-revisions --target <target> <label>
```

For large repositories, start browsing under a known subtree:

```bash
duplicacy-backup restore select --target <target> --path-prefix <relative-path> <label>
```

Restore into the drill workspace only:

```bash
duplicacy-backup restore run --target <target> --revision <id> --path <relative-path-or-pattern> --yes <label>
```

Use a snapshot-relative file path for one file, or a Duplicacy pattern such as
`phillipmcmahon/code/*` for a directory subtree. In `restore select`, use the
arrow keys to move through the tree, `Right` and `Left` to expand and collapse
directories, `Space` to toggle the current file or subtree, and `g` to
generate the matching primitive `restore run` commands. Use `q` at prompts or
inside the picker to cancel before execution. During an active restore,
`Ctrl-C` cancels Duplicacy, keeps the drill workspace, does not delete restored
files, and reports progress.

Do not restore directly over the live source path as the first step. Restore
elsewhere, inspect the result, and copy back only the intended files or
directories.

See also:

- [Restore onto a new NAS](new-nas-restore.md)
- [Restore drills](restore-drills.md)
- [Operator cheat sheet](cheatsheet.md#restore-drills)

## Update Attestation Fails on an Expected Release

If you use `--attestations required`, the updater must be able to run GitHub
CLI attestation verification before extraction and install. On unattended
systems this also means `gh` must be installed and logged in for the account
running the scheduled update.

Use `--attestations auto` if you want scheduled updates to verify attestations
when `gh` is available but continue with checksum verification when
authenticated attestation checks are unavailable. Keep `required` when you want
the update to stop unless attestation verification succeeds.

See also:

- [Update trust model](update-trust-model.md)
- [Operations](operations.md#upgrade-and-rollback)

## Notification Test Works but Scheduled Events Do Not Notify

`notify test` proves provider resolution, optional provider authentication,
and delivery to the configured destination. It does not prove that a real
backup, prune, health, or update event matched your notification policy.

Check the configured scope:

- label runtime and health notifications use `[health.notify]`
- update notifications use the global `[update.notify]` file
- runtime notifications require the operation in `send_for`
- notifications from interactive TTY runs need `interactive = true`
- success events are quiet unless explicitly enabled where supported

If the scheduled command failed before notification config could be read, rely
on Synology scheduled-task output or email as the fallback signal.

See also:

- [Configuration and secrets](configuration.md#notifications)
- [Operator cheat sheet](cheatsheet.md#notifications-and-secrets)

## Config Validation Shows Not Checked

`Not checked` means the validation step is conditional or could not be run with
the selected inputs. It is not the same as a failed check.

For `config validate`, an accessible configured `source_path` always receives
the Btrfs source check. If that path is not on Btrfs or is not a subvolume root,
validation fails because backups require snapshot consistency.

Non-root validation is the default. In the v9 profile model, Btrfs source
validation and secrets loading are designed to run as the operator user when the
selected paths are accessible. Path-based local repository readiness is the
exception: that probe requires `sudo` because local repository metadata is
root-protected by design.

See also:

- [Configuration and secrets](configuration.md#conditional-validation)
- [Operator cheat sheet](cheatsheet.md#config-and-validation)

## Log File Permission Is Denied

Runtime logs are written under
`$HOME/.local/state/duplicacy-backup/logs` by default. A log permission error
usually means the current user cannot write to their own state directory or an
explicit log path has been configured with restrictive permissions.

Use `sudo` only for commands that need root-level OS operations. Scheduled
backup jobs run with `sudo` from the operator account; health, restore,
diagnostics, object-storage prune, and notification checks can run as the
operator user when their config, secrets, state, log, lock, storage, and
workspace paths are accessible.

## Direct Root Execution Is Ambiguous

If a command fails with `direct root execution is ambiguous`, it was started as
root without complete sudo metadata. Run the command as the operator user, or
for root-required operations run it with `sudo` from that operator account.

For rare expert direct-root use, pass the intended profile roots explicitly:

```bash
XDG_STATE_HOME=/var/services/homes/<user>/.local/state \
  duplicacy-backup <command> \
    --config-dir /var/services/homes/<user>/.config/duplicacy-backup \
    --secrets-dir /var/services/homes/<user>/.config/duplicacy-backup/secrets \
    ...
```

This keeps logs, state, locks, config, and secrets from silently resolving under
`/root`.

See also:

- [Operator cheat sheet](cheatsheet.md)
- [Privilege model](privilege-model.md#ambiguous-direct-root-guard)
- [Workflow and scheduling](workflow-scheduling.md#synology-task-scheduler-setup)

## Update Reinstall or Rollback Looks Wrong

`update` keeps the newly activated binary and one previous binary by default.
Use `--keep <count>` when you want a different local rollback window, and use
`--force` when you need to reinstall the current version.

Start with the non-mutating rollback inspection:

```bash
/usr/local/bin/duplicacy-backup rollback --check-only
```

If the previous retained version is the right target, activate it with:

```bash
sudo /usr/local/bin/duplicacy-backup rollback --yes
```

Scheduled tasks should call the stable command path:

```text
/usr/local/bin/duplicacy-backup
```

If an update reports path or layout errors, confirm that the command is being
invoked through the installed symlink rather than from an extracted test
package or a deleted working directory.

If rollback reports that no previous retained version exists, the local update
retention window has kept only the active binary. Install the desired release
tarball manually, or increase future update retention with `update --keep`.

See also:

- [Operations](operations.md#upgrade-and-rollback)
- [Update trust model](update-trust-model.md)
