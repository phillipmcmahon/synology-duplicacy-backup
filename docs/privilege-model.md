# Privilege Model

`duplicacy-backup` defaults to non-root operation. Root is required only when
the command performs an operating-system action that a normal user cannot do
safely.

## Default User Profile

By default, runtime files live under the user running the command:

```text
$HOME/.config/duplicacy-backup/<label>-backup.toml
$HOME/.config/duplicacy-backup/secrets/<label>-secrets.toml
$HOME/.local/state/duplicacy-backup/logs/
$HOME/.local/state/duplicacy-backup/state/
$HOME/.local/state/duplicacy-backup/locks/
```

`XDG_CONFIG_HOME` and `XDG_STATE_HOME` are honoured when set.

Secrets files must use mode `0600`. For normal non-root commands, the file
must be owned by the user running `duplicacy-backup`. For root-required commands
started with `sudo` from the operator account, the same operator-owned secrets
file is accepted so root-required operations do not require a separate root copy
of the runtime profile. The secrets directory should use mode `0700`.

## Migrating Legacy Root-Owned Files

For one-time migration from the old root-required defaults, follow
[`v8-migration.md`](v8-migration.md). The migration helper is packaged in the
release tarball; `duplicacy-backup update` does not install it into the managed
`current` directory.

By default the helper migrates:

```text
/usr/local/lib/duplicacy-backup/.config/*.toml -> $HOME/.config/duplicacy-backup/
/root/.secrets/*.toml                         -> $HOME/.config/duplicacy-backup/secrets/
```

It creates destination directories with mode `0700`, preserves source
timestamps where supported, sets migrated TOML files to `0600`, and chowns
them to the target user and that user's primary group when run as root. It
copies by default; `--move` removes each legacy source file after a successful
copy.

## Root-Required Commands

These commands require root because of the work they perform:

| Command | Why root is required |
|---|---|
| `backup` | Creates a btrfs snapshot and needs complete source-tree read access. |
| `prune` / `cleanup-storage` for path-based filesystem storage | Mutates a protected local backup repository. Local repository chunks and snapshots should remain OS-protected and policy-managed. |
| `fix-perms` | Runs ownership and permission repair with `chown` and `chmod`. |
| `update --yes` | Activates a managed install under system-owned paths. |
| `rollback --yes` | Changes managed install symlinks under system-owned paths. |

For root-required commands, keep the canonical runtime profile under the
operator user and invoke through `sudo` from that operator account:

```bash
sudo duplicacy-backup backup --target onsite-usb homes
```

When invoked this way with normal sudo metadata, default config, secrets, logs,
state, and lock paths resolve to the sudoing operator profile, and
`duplicacy-backup` accepts `0600` secrets owned by that operator account.

If you log in directly as `root`, or run with a stripped environment that lacks
complete sudo metadata, defaults resolve under `/root`. Either use a root-owned
runtime profile under `/root`, run `sudo` from the operator account, or pass
explicit `--config-dir` and `--secrets-dir` values so the intended profile is
unambiguous.

## Non-Root-Capable Commands

These commands are intended to run as a normal user when their selected config,
secrets, storage, workspace, state, log, and lock paths are accessible:

| Command | Notes |
|---|---|
| `restore select` / `restore run` / `restore plan` / `restore list-revisions` | Restores write only to a drill workspace, never the live source path. |
| `health status` / `health verify` | Requires access to Duplicacy storage and runtime paths. `health verify` verifies repository integrity and does not perform Btrfs backup-readiness checks. |
| `health doctor` | Requires access to Duplicacy storage and runtime paths. Its Btrfs backup-readiness check uses unprivileged `stat` probes to confirm the source is on Btrfs and is a subvolume root. |
| `prune` / `cleanup-storage` for object or remote storage | Mutates storage through Duplicacy credentials. The authority boundary is the credential, not OS root. |
| `prune --dry-run` | Preview-only; may run non-root when the repository is readable. |
| `config` / `diagnostics` / `notify` | Reads config/secrets and writes only normal command output or notifications. |
| `update --check-only` / `rollback --check-only` | Inspects managed install state without changing it. |

## Repository Mutation Boundary

There are two repository access models:

- Path-based filesystem repositories are protected operating-system resources.
  Backups normally write them as root, and actual prune or cleanup-storage
  mutation must also run as root. This prevents ordinary users from inspecting,
  deleting, or rewriting chunks and snapshot metadata outside the tool's
  retention policy.
- Object and remote repositories are governed by credentials. If the operator
  owns the storage keys and the backend accepts them, prune and cleanup-storage
  can run as that operator user.

This distinction is intentional. Do not chown local backup repositories to make
routine mutation easier unless you have deliberately chosen a different
site-specific security model.

If a command fails with a path permission error, fix the selected path
ownership/mode or pass explicit `--config-dir`, `--secrets-dir`, `--workspace`,
or `--workspace-root` values. Do not use `sudo` unless the command is actually
performing a root-required operation.
