# Documentation Index

Use this page as the front door for the operator documentation. The docs are
organised by task rather than by filename so a new operator can follow a
clear path without reading internals first.

## Operator Path

| Need | Start here |
|---|---|
| Confirm production assumptions before installing | [Requirements](requirements.md) |
| Install, upgrade, rollback, or verify release assets | [Operations](operations.md) |
| Create or review label, target, health, notification, and secrets config | [Configuration and secrets](configuration.md) |
| Run common day-to-day commands | [Operator cheat sheet](cheatsheet.md) |
| Build Synology Task Scheduler jobs | [Workflow and scheduling](workflow-scheduling.md) |
| Diagnose failed runs or unclear status output | [Troubleshooting](troubleshooting.md) |
| Practise full or selective restores on an existing NAS | [Restore drills](restore-drills.md) |
| Restore onto a replacement NAS | [Restore onto a new NAS](new-nas-restore.md) |
| Check exact command syntax and flags | [CLI reference](cli.md) |
| Understand update integrity checks and attestations | [Update trust model](update-trust-model.md) |
| Understand root, sudo, and repository access rules | [Privilege model](privilege-model.md) |

## Maintainer Path

| Need | Start here |
|---|---|
| Prepare or verify a release | [Release playbook](release-playbook.md) |
| Understand the site-specific NAS release mirror | [Release mirror](release-mirror.md) |
| Reproduce the Linux validation and packaging environment | [Linux environment](linux-environment.md) |
| Capture and review UI surface output | [UI surface smoke testing](ui-surface-smoke.md) |
| Understand the high-level design | [Architecture](architecture.md) |
| Follow the detailed internal runtime flow | [How it works](how-it-works.md) |
| Review workflow package split criteria | [Workflow boundary review](workflow-boundary-review.md) |

## Suggested First-Time Flow

1. Read [Requirements](requirements.md) to confirm the NAS and storage model.
2. Install the binary using [Operations](operations.md).
3. Create the first label config with [Configuration and secrets](configuration.md).
4. Validate and run the first backup using [Operator cheat sheet](cheatsheet.md).
5. Add scheduled tasks with [Workflow and scheduling](workflow-scheduling.md).
6. Prove recovery with [Restore drills](restore-drills.md).
