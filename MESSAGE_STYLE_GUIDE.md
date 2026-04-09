# Message Style Guide

Conventions for operator-facing messages in `synology-duplicacy-backup`.

---

## Ownership Rule

**Internal packages never log directly.** All operator-facing messages are
emitted by the coordinator (`cmd/duplicacy-backup/main.go`).

| Layer | Responsibility |
|---|---|
| `internal/*` packages | Return structured errors and raw output |
| Coordinator (`main.go`) | Format, log, and present all messages |

---

## Sentence Case with Punctuation

All log messages use **sentence case** (first word capitalised, rest lowercase
unless proper nouns) and end with a **full stop** (period).

```
✅  "Backup completed successfully."
✅  "Prune preview: 5 revisions would be deleted."
✅  "Snapshot created at /volume1/homes-20260409-120000."

❌  "Backup Completed Successfully"     ← title case
❌  "backup completed successfully"     ← no capitalisation
❌  "Backup completed successfully"     ← missing full stop
```

---

## Phase Messages

Operations are bracketed by phase-start and phase-end messages:

```
[INFO] Running backup phase.
[INFO] ... (command output piped through logger) ...
[INFO] Backup phase completed.
```

Phase names: `backup`, `prune`, `deep prune`, `fix permissions`, `snapshot
creation`, `snapshot deletion`, `cleanup`.

---

## Dry-Run Messages

Dry-run messages use the prefix `[DRY-RUN]` and describe what *would* happen:

```
[INFO] [DRY-RUN] Would create snapshot at /volume1/homes-20260409-120000.
[INFO] [DRY-RUN] Would run: duplicacy backup -threads 4
```

---

## Error Messages

Errors logged by the coordinator include context about *what* failed and *why*:

```
[ERROR] Snapshot creation failed for /volume1/homes: btrfs returned exit code 1.
[ERROR] Backup failed: duplicacy exited with status 2.
```

---

## Structured Error Types

The `internal/errors` package provides typed errors with rich context:

| Type | Phase | Use |
|---|---|---|
| `BackupError` | backup | Duplicacy backup failures |
| `PruneError` | prune | Duplicacy prune failures |
| `SnapshotError` | snapshot | BTRFS snapshot create/delete/check failures |
| `PermissionsError` | permissions | Permission fixing failures |
| `ConfigError` | config | Configuration loading/parsing failures |
| `SecretsError` | secrets | Secrets file loading/validation failures |
| `LockError` | lock | Lock acquisition failures |

Each error carries:

- **Phase** — which operation phase the error belongs to
- **Field** — the specific resource (path, label, etc.)
- **Cause** — the underlying error
- **Context** — a `map[string]string` of additional key-value context

### Using structured errors in the coordinator

```go
import apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"

err := btrfs.CreateSnapshot(runner, src, dst, false)
if err != nil {
    var snapErr *apperrors.SnapshotError
    if errors.As(err, &snapErr) {
        log.Error("Snapshot creation failed for %s: %v.", snapErr.Field, snapErr.Cause)
    }
    return err
}
```

---

## Command Output Piping

When internal packages return command stdout/stderr, the coordinator pipes
non-empty output through the logger:

```go
stdout, stderr, err := a.dup.RunBackup(threads)
printCommandOutput(a.log, stdout, stderr)
```

This ensures all command output appears in structured log files with timestamps.
