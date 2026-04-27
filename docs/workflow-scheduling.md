# Workflow and Scheduling

## Goal

Use this guide to create simple, repeatable Synology Task Scheduler jobs for
`duplicacy-backup`.

The safest model is:

- run scheduled tasks as the operator user by default
- prefix only root-required commands with `sudo -n`
- keep one scheduled task focused on one operation
- use cadence slots such as daily, weekly, and monthly rather than a large
  custom schedule for every label

## Scheduling Rules

Use these rules before creating tasks:

- `backup` always uses `sudo -n` because it needs btrfs snapshot access and
  full source read access, regardless of whether the target storage is local or
  remote.
- `prune` uses `sudo -n` for path-based local filesystem storage because the
  repository files are OS-protected resources.
- `prune` runs as the operator user for object or remote storage when the
  operator owns the config, secrets, state, log, lock, and credential access.
- `health status` / `doctor` / `verify` and restore revision/list/run/select
  commands use `sudo -n` for path-based local repositories because repository
  metadata is root-protected.
- `health`, `diagnostics`, restore planning, and object or remote repository
  restore commands run as the operator user.
- `cleanup-storage` is manual or exceptional maintenance, not a routine
  schedule.
- `prune --force` is manual only. Do not schedule it routinely.

After migrating runtime files into an operator profile, avoid scheduling
non-root-capable tasks as `root` out of habit. Profile-using commands started
from direct root are rejected unless the intended config, secrets, and state
roots are explicit. Schedule the task as the operator user instead, then use
`sudo -n` only for the exact root-required command.

For scheduled root-required commands, add a narrow `/etc/sudoers.d` rule that
allows only the exact `duplicacy-backup` commands used by the scheduler. Do not
grant passwordless sudo for all commands, and avoid granting passwordless sudo
for the whole binary unless that is an explicit site policy decision. See
[`privilege-model.md`](privilege-model.md#least-privilege-sudo-for-scheduled-tasks)
for a generalized sudoers template.

## Task Naming

Use a label-first pattern so tasks sort cleanly in DSM:

```text
<Label> Backup <Target>
<Label> Prune <Target>
<Label> Health Status <Target>
<Label> Health Doctor <Target>
<Label> Health Verify <Target>
```

Examples:

```text
Homes Backup Onsite USB
Homes Backup Offsite Storj
Homes Prune Onsite USB
Homes Health Status Offsite Storj
```

## Cadence Template

Start with these slots, then put the relevant labels and targets into each one.

### Frequent Backup Slot

Use for important local or fast targets.

Recommended cadence:

- daily, repeat every 6 hours where appropriate
- keep enough spacing before prune or verify jobs

Command template:

```bash
sudo -n /usr/local/bin/duplicacy-backup backup --target <target> <label>
```

Example entries:

```text
homes      -> onsite-usb      -> every 6 hours
plexaudio  -> onsite-usb      -> daily
plexvideo  -> onsite-usb      -> daily
```

### Daily Backup Slot

Use for slower offsite, remote, or object-storage targets.

Recommended cadence:

- daily
- run after the main onsite backup window

Command template:

```bash
sudo -n /usr/local/bin/duplicacy-backup backup --target <target> <label>
```

Example entries:

```text
homes      -> offsite-storj   -> daily
plexaudio  -> offsite-storj   -> daily
```

### Weekly Backup Slot

Use for low-change labels or large data sets that do not need daily backup.

Command template:

```bash
sudo -n /usr/local/bin/duplicacy-backup backup --target <target> <label>
```

Example entries:

```text
iso        -> onsite-usb      -> weekly
```

### Prune Slot

Schedule prune separately from backup.

Recommended cadence:

- daily for busy local repositories
- weekly or twice weekly for slower offsite repositories

Command templates:

```bash
# Path-based local filesystem repository
sudo -n /usr/local/bin/duplicacy-backup prune --target <local-path-target> <label>

# Object or remote repository
/usr/local/bin/duplicacy-backup prune --target <object-or-remote-target> <label>
```

Example entries:

```text
homes      -> onsite-usb      -> daily, with sudo -n
homes      -> offsite-storj   -> weekly, no sudo
```

### Daily Health Slot

Use this for lightweight visibility that a target is reachable and the latest
revision signal is sensible.

Command templates:

```bash
# Path-based local repositories
sudo -n /usr/local/bin/duplicacy-backup health status --target <target> <label>

# Object or remote repositories
/usr/local/bin/duplicacy-backup health status --target <target> <label>
```

Example entries:

```text
homes      -> onsite-usb      -> daily
homes      -> offsite-storj   -> daily
```

### Weekly Doctor Slot

Use this for a deeper repository and configuration check.

Command templates:

```bash
# Path-based local repositories
sudo -n /usr/local/bin/duplicacy-backup health doctor --target <target> <label>

# Object or remote repositories
/usr/local/bin/duplicacy-backup health doctor --target <target> <label>
```

Example entries:

```text
homes      -> onsite-usb      -> weekly
homes      -> offsite-storj   -> weekly
```

### Monthly Verify Slot

Use this for storage-integrity verification. It can take longer than status or
doctor checks, so give it its own window.

Use `sudo -n` for path-based local repositories because their Duplicacy metadata
is root-protected. Object and remote repository verification can run as the
operator user without `sudo`.

Command templates:

```bash
# Path-based local repositories
sudo -n /usr/local/bin/duplicacy-backup health verify --target <target> <label>

# Object or remote repositories
/usr/local/bin/duplicacy-backup health verify --target <target> <label>
```

Example entries:

```text
homes      -> onsite-usb      -> monthly
homes      -> offsite-storj   -> monthly
```

## Synology Task Scheduler Setup

For each task:

1. Open `Control Panel` -> `Task Scheduler`.
2. Create `Triggered Task` -> `User-defined script`.
3. Choose the operator user as the run-as user.
4. Enter one command from the relevant cadence slot.
5. Use `sudo -n` only where the cadence slot says the command is root-required.
6. Keep one operation per task.

If a task fails with a sudo password prompt or permission error, do not switch
the whole task to run as `root` as a shortcut. Fix the targeted sudoers rule or
the operator profile permissions instead.

## Manual Or Exceptional Maintenance

### Cleanup Storage

Do not make `cleanup-storage` part of the normal recurring schedule.

Use it only for explicit maintenance when no other client is writing to the
repository:

```bash
# Path-based local filesystem repository
sudo -n /usr/local/bin/duplicacy-backup cleanup-storage --target <local-path-target> <label>

# Object or remote repository
/usr/local/bin/duplicacy-backup cleanup-storage --target <object-or-remote-target> <label>
```

### Forced Prune

Do not schedule `prune --force` routinely.

Use it only as an explicit operator decision when safe-prune thresholds are
intentionally being overridden:

```bash
sudo -n /usr/local/bin/duplicacy-backup prune --force --target <target> <label>
```

## Operational Notes

- Keep DSM scheduler email enabled for raw task failures.
- Use native `ntfy` for richer degraded, unhealthy, and selected runtime alerts.
- Move a later task if it routinely overlaps another task touching the same
  label and target.
- Prefer changing cadence before combining unrelated commands into one script.

## Supported Boundary

This guide deliberately does not automate Synology Task Scheduler creation.

Reason:

- DSM does not provide a straightforward supported CLI for creating tasks.
- Unsupported internal automation would make the workflow guidance more brittle.

For now, the supported model is:

- documented task naming
- documented cadence slots
- documented script commands
- manual creation in the DSM UI
