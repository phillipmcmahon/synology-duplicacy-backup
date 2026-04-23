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
sudo duplicacy-backup config validate --target <target> <label>
sudo duplicacy-backup health status --target <target> <label>
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
command such as prune, cleanup, update, or fix-perms, it can fail even when the
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

## Health Verify Is Degraded or Unhealthy

`health verify` uses health-specific exit codes: `0` for healthy, `1` for
degraded, and `2` for unhealthy. An unhealthy result does not always mean a
new backup just failed; it can also mean integrity, recency, or repository
state no longer meets policy.

Run a JSON summary when you need precise automation detail:

```bash
sudo duplicacy-backup health verify --json-summary --target <target> <label>
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

See also:

- [Configuration and secrets](configuration.md#secrets)
- [Duplicacy storage backends](https://github.com/gilbertchen/duplicacy/wiki/Storage-Backends)

## I Need To Restore Data

The wrapper restores only into a separate drill workspace. It does not copy
restored data back to the live source path.

Use `restore plan` to gather that context and print safe next-step commands:

```bash
sudo duplicacy-backup restore plan --target <target> <label>
```

Use `restore prepare` when you want the wrapper to create the separate drill
workspace and write Duplicacy preferences there:

```bash
sudo duplicacy-backup restore prepare --target <target> <label>
```

List revisions and inspect the specific revision before restoring:

```bash
sudo duplicacy-backup restore revisions --target <target> <label>
sudo duplicacy-backup restore files --target <target> --revision <id> --path <relative-path> <label>
```

Use the guided picker when you want command generation help without executing a
restore:

```bash
sudo duplicacy-backup restore select --target <target> <label>
```

For large repositories, start browsing under a known subtree:

```bash
sudo duplicacy-backup restore select --target <target> --path-prefix <relative-path> <label>
```

If the workspace is already prepared and you want guarded execution through
`restore run`, add `--execute` and confirm the generated command:

```bash
sudo duplicacy-backup restore select --target <target> --execute <label>
```

Restore into the prepared workspace only:

```bash
sudo duplicacy-backup restore run --target <target> --revision <id> --path <relative-path-or-pattern> --workspace /volume1/restore-drills/<label>-<target> --yes <label>
```

Use a snapshot-relative file path for one file, or a Duplicacy pattern such as
`phillipmcmahon/code/*` for a directory subtree.

Do not restore directly over the live source path as the first step. Restore
elsewhere, inspect the result, and copy back only the intended files or
directories.

See also:

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

`Not checked` means the current user could not safely perform that validation
step, usually because the check needs root-only paths or permissions. It is not
the same as a failed check.

Non-root validation can still be useful for syntax and some path checks, but
run the same command with `sudo` before treating the config as fully validated
for scheduled NAS operation.

See also:

- [Configuration and secrets](configuration.md#conditional-validation)
- [Operator cheat sheet](cheatsheet.md#config-and-validation)

## Log File Permission Is Denied

Installed NAS runtime and health commands normally write logs under `/var/log`
and should be run with `sudo`. A normal-user run can fail before the operation
starts if it cannot open the log file.

Use normal-user commands only where they are designed to avoid protected
runtime paths, such as `config paths`, `diagnostics`, `update --check-only`,
`rollback --check-only`, and dry-run notification tests. For scheduled backup,
prune, health, cleanup, and permission-fix jobs, use the installed command with
root privileges.

See also:

- [Operator cheat sheet](cheatsheet.md)
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
