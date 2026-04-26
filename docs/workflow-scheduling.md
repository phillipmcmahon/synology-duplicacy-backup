# Workflow and Scheduling

## Goal

Use this guide for the recommended operational workflow when running
`duplicacy-backup` under Synology Task Scheduler.

It focuses on supported, repeatable operator practice:

- separate scheduled tasks rather than unsupported DSM automation
- stable task naming
- sensible timing and spacing
- clear distinction between routine operations and occasional maintenance

## Scheduling Philosophy

The CLI uses first-class runtime commands, and scheduled operations are easiest
to understand and troubleshoot when each task has one clear purpose.

Recommended approach:

- schedule `backup` as its own task
- schedule `prune` as its own task
- schedule health commands separately from backup jobs
- run path-based filesystem repository prune and cleanup tasks as root
- treat `cleanup-storage` as manual or exceptional maintenance

Why this works well:

- failures are easier to attribute
- runtime windows are easier to tune per target
- scheduler email and notification signals stay clearer
- slow offsite work does not force the same cadence as fast onsite work

## Naming Convention

Use a label-first task naming pattern so scheduled jobs sort cleanly and scale
across multiple labels.

Recommended names:

- `<Label> Backup Onsite`
- `<Label> Backup Offsite`
- `<Label> Prune Onsite`
- `<Label> Prune Offsite`
- `<Label> Fix Perms Onsite`
- `<Label> Health Status Onsite`
- `<Label> Health Status Offsite`
- `<Label> Health Doctor Onsite`
- `<Label> Health Doctor Offsite`
- `<Label> Health Verify Onsite`
- `<Label> Health Verify Offsite`

Examples:

- `Homes Backup Onsite`
- `Homes Backup Offsite`
- `Plexaudio Backup Onsite`
- `Plexvideo Backup Onsite`

## Timing Rules

Use these guidelines before choosing exact times:

- treat the most important label and target as the priority workload
- keep onsite and offsite cadences independent
- leave some spacing between tasks that touch the same target
- use Synology's built-in repeat scheduling where it fits naturally
- do not combine tasks just to reduce the number of scheduler entries

Good candidates for repeat-every scheduling:

- frequent onsite backups such as every 6 hours

Good candidates for separate scheduled entries:

- offsite backups
- prune
- health status
- health doctor
- health verify

Some overlap is acceptable, but prefer moving the later task rather than
merging unrelated operations into one job.

## What To Schedule Routinely

### Backup

This is the main routine workload.

Recommended pattern:

- fast onsite targets more frequently
- slower offsite or higher-latency storage targets less frequently

`location` is the scheduling signal: a local S3-compatible service on your LAN
can use an onsite cadence if it is fast and reliable enough, while a remote
filesystem mount should still be treated like offsite work.

Examples:

- `homes` onsite every 6 hours
- `homes` offsite daily
- `plexaudio` onsite daily
- `plexaudio` offsite daily
- `plexvideo` onsite daily
- `iso` onsite weekly

### Prune

Schedule prune separately from backup.

Recommended pattern:

- onsite prune daily if that repository is used heavily
- offsite prune less often if runtime or storage churn is a concern

Examples:

- `homes` onsite daily
- `homes` offsite twice weekly

### Health

Schedule health separately from backup jobs.

Recommended pattern:

- `health status` daily
- `health doctor` weekly
- `health verify` monthly

This keeps backup execution separate from confidence checks and makes scheduler
output easier to interpret.

### Fix Permissions

Only schedule this for path-based Duplicacy storage targets.

Recommended pattern:

- weekly
- or after manual restore or repository maintenance activity

Do not schedule it for URL-like storage targets such as S3, Storj, B2, RustFS,
or MinIO.

## What Not To Schedule Routinely

### Cleanup Storage

Do not make `cleanup-storage` part of the normal recurring schedule.

Treat it as:

- manual maintenance
- exceptional cleanup
- an operation to run only when no other client is actively writing

### Forced Prune

Do not schedule `prune --force` routinely.

Use it only as an explicit operator decision when safe-prune thresholds are
intentionally being overridden.

## Synology Task Scheduler Setup

For each task:

1. Open `Control Panel` -> `Task Scheduler`
2. Create `Triggered Task` -> `User-defined script`
3. Choose the run-as user:
   - use `root` for `backup` and path-based filesystem repository `prune` or
     `cleanup-storage`
   - use the operator user for health, diagnostics, restore checks, and
     object/remote repository prune or cleanup when that user owns the config,
     secrets, state, log, lock, and storage access paths
4. Use `/usr/local/bin/duplicacy-backup`
5. Keep one task per operation

After migrating runtime files into an operator profile, avoid scheduling
non-root-capable tasks as `root` out of habit. A root scheduled health or prune
task resolves `$HOME` as `/root` and will look under
`/root/.config/duplicacy-backup` unless you pass explicit `--config-dir` and
`--secrets-dir` values. Prefer running those tasks as the operator user that
owns the migrated profile.

Example commands:

```bash
/usr/local/bin/duplicacy-backup backup --target onsite-usb homes
/usr/local/bin/duplicacy-backup backup --target offsite-storj homes
/usr/local/bin/duplicacy-backup prune --target onsite-usb homes
/usr/local/bin/duplicacy-backup health status --target onsite-usb homes
/usr/local/bin/duplicacy-backup health verify --json-summary --target offsite-storj homes
```

## Worked Example

This is a practical scheduling model for:

- `homes`
- `iso`
- `plexaudio`
- `plexvideo`

### Homes

- `Homes Backup Onsite`
  Daily, start `01:00`, repeat every `6 hours`
- `Homes Backup Offsite`
  Daily at `02:30`
- `Homes Prune Onsite`
  Daily at `04:30`
- `Homes Prune Offsite`
  Tuesday and Friday at `04:45`
- `Homes Fix Perms Onsite`
  Sunday at `05:00`
- `Homes Health Status Onsite`
  Daily at `08:00`
- `Homes Health Status Offsite`
  Daily at `08:10`
- `Homes Health Doctor Onsite`
  Sunday at `08:30`
- `Homes Health Doctor Offsite`
  Sunday at `08:45`
- `Homes Health Verify Onsite`
  First Sunday at `09:00`
- `Homes Health Verify Offsite`
  First Sunday at `10:00`

### Iso

- `Iso Backup Onsite`
  Saturday at `01:30`

### Plexaudio

- `Plexaudio Backup Onsite`
  Daily at `03:30`
- `Plexaudio Backup Offsite`
  Daily at `05:30`

### Plexvideo

- `Plexvideo Backup Onsite`
  Daily at `06:30`

## Quick Reference Table

| Task Name | Frequency | Time | Command |
|---|---|---|---|
| `Homes Backup Onsite` | Daily | Start `01:00`, repeat every `6 hours` | `/usr/local/bin/duplicacy-backup backup --target onsite-usb homes` |
| `Homes Backup Offsite` | Daily | `02:30` | `/usr/local/bin/duplicacy-backup backup --target offsite-storj homes` |
| `Homes Prune Onsite` | Daily | `04:30` | `/usr/local/bin/duplicacy-backup prune --target onsite-usb homes` |
| `Homes Prune Offsite` | Weekly | `Tuesday, Friday` at `04:45` | `/usr/local/bin/duplicacy-backup prune --target offsite-storj homes` |
| `Homes Health Status Onsite` | Daily | `08:00` | `/usr/local/bin/duplicacy-backup health status --target onsite-usb homes` |
| `Homes Health Status Offsite` | Daily | `08:10` | `/usr/local/bin/duplicacy-backup health status --target offsite-storj homes` |
| `Homes Health Doctor Onsite` | Weekly | `Sunday` at `08:30` | `/usr/local/bin/duplicacy-backup health doctor --target onsite-usb homes` |
| `Homes Health Doctor Offsite` | Weekly | `Sunday` at `08:45` | `/usr/local/bin/duplicacy-backup health doctor --target offsite-storj homes` |
| `Homes Health Verify Onsite` | Monthly | `First Sunday` at `09:00` | `/usr/local/bin/duplicacy-backup health verify --target onsite-usb homes` |
| `Homes Health Verify Offsite` | Monthly | `First Sunday` at `10:00` | `/usr/local/bin/duplicacy-backup health verify --target offsite-storj homes` |
| `Iso Backup Onsite` | Weekly | `Saturday` at `01:30` | `/usr/local/bin/duplicacy-backup backup --target onsite-usb iso` |
| `Plexaudio Backup Onsite` | Daily | `03:30` | `/usr/local/bin/duplicacy-backup backup --target onsite-usb plexaudio` |
| `Plexaudio Backup Offsite` | Daily | `05:30` | `/usr/local/bin/duplicacy-backup backup --target offsite-storj plexaudio` |
| `Plexvideo Backup Onsite` | Daily | `06:30` | `/usr/local/bin/duplicacy-backup backup --target onsite-usb plexvideo` |

## Operational Notes

- keep scheduler email enabled for raw task failures
- use native `ntfy` for richer degraded, unhealthy, and selected runtime alerts
- if repeated alerts become noisy, adjust task frequency or receiving-system
  suppression before changing the core workflow model
- if a later task routinely overlaps, move that task rather than combining jobs

## Supported Boundary

This guide deliberately does not automate Synology Task Scheduler creation.

Reason:

- DSM does not provide a straightforward supported CLI for creating tasks
- unsupported internal automation would make the workflow guidance more brittle

For now, the supported model is:

- documented task naming
- documented timing guidance
- documented script commands
- manual creation in the DSM UI
