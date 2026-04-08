# `--fix-perms` Implementation Analysis (Revised)

## Correct Understanding of `--fix-perms`

**Purpose**: Normalise ownership and permissions on the **local backup DESTINATION** — i.e., the directory where backup data is stored (e.g., `/volume2/backups/homes`), NOT the source being backed up.

**Rationale**: The backup runs as root, so files written to the local backup destination end up root-owned. `--fix-perms` corrects this to `LOCAL_OWNER:LOCAL_GROUP` for security and manageability.

**Scope**:
- Only meaningful for **local** backup destinations (filesystem paths)
- Has no meaning for remote destinations (S3, etc.) — there are no POSIX permissions on cloud storage
- Should apply `LOCAL_OWNER` and `LOCAL_GROUP` from the `[local]` config section

---

## Current Implementation Summary

### Flag Definition & Parsing
- **Defined**: `main.go:80` — `fixPerms bool` field in the `flags` struct
- **Parsed**: `main.go:576-577` — `case "--fix-perms": f.fixPerms = true`
- **Not treated as a "mode"**: It's a modifier boolean, not a mode like `--backup`/`--prune`/`--prune-deep`

### Execution Point
- **Line 552-559** in `main.go`:
  ```go
  // Fix permissions for local mode
  if !f.remoteMode && f.fixPerms {
      if err := permissions.Fix(log, backupTarget, cfg.LocalOwner, cfg.LocalGroup, f.dryRun); err != nil {
          ...
      }
  }
  ```
- Calls `permissions.Fix()` in `internal/permissions/permissions.go`
- `permissions.Fix()` does: `chown -R owner:group target` + `chmod 770` dirs / `chmod 660` files
- **It does have logging**: "Starting local ownership and permission normalisation" and "Completed..." messages

### What `backupTarget` resolves to
- `backupTarget = joinDestination(cfg.Destination, backupLabel)` (line 348)
- For local mode with `DESTINATION=/volume2/backups`, this becomes `/volume2/backups/homes`
- **This IS the correct target** — the local backup destination directory

---

## Issues Identified

### Issue 1: `--fix-perms` with `--remote` — WARNING but no hard stop
**Current behaviour** (line 152-154):
```go
if f.fixPerms && f.remoteMode {
    fmt.Fprintln(os.Stderr, "[WARNING] --fix-perms has no effect with --remote")
}
```
- Only prints a **warning** to stderr, then continues execution
- The actual fix-perms code at line 553 is guarded by `!f.remoteMode`, so it correctly won't execute
- **Problem**: The warning is misleading — the program continues to do a full backup/prune with `--remote`, silently ignoring `--fix-perms`. The user might think fix-perms ran. This should be a **hard error** (exit 1), not a warning.

### Issue 2: `--fix-perms` alone always triggers a backup (THE MAIN BUG)
**Root cause** (line 604-607):
```go
// Default mode is backup
if f.mode == "" {
    f.mode = "backup"
}
```
- `--fix-perms` is a **modifier**, not a **mode**
- Running `duplicacy-backup --fix-perms homes` results in `f.mode = ""` → defaults to `"backup"`
- So `doBackup = true` at line 138: `doBackup := f.mode == "backup"`
- This means the program:
  1. Creates a btrfs snapshot (line 431)
  2. Sets up duplicacy working environment (line 439)
  3. Runs a full backup (line 474)
  4. **Then** fixes permissions (line 553)
  5. Cleans up snapshot in defer (line 196)

- **The user expected `--fix-perms` to be standalone** — just fix permissions and exit. Instead, it silently performs a full backup first.

### ~~Issue 3: Fix-perms targets the wrong path~~ → **NOT AN ISSUE**
**Previous analysis was incorrect.** The previous analysis claimed `backupTarget` was the wrong path and that `snapshotSource` should be used instead. This was wrong.

**Correct analysis**:
- `backupTarget` = `joinDestination(cfg.Destination, backupLabel)` → e.g., `/volume2/backups/homes`
- This IS the correct target — the local backup destination where backup files are stored
- The purpose of `--fix-perms` is to fix ownership/permissions on these backup files, not on the source data
- For local mode, `cfg.Destination` is a filesystem path (e.g., `/volume2/backups`), so `backupTarget` resolves to a valid local path
- **The current target path is correct.** ✅

### Issue 3 (actual): No visible operation mode output for fix-perms only
- The operation mode section (lines 421-427) only shows "Backup only", "Prune deep", or "Prune safe"
- When `--fix-perms` is the sole intended operation, there is no "Fix permissions only" output
- Currently, because of Issue 2, it shows "Backup only" which is misleading

### Issue 4: Unnecessary setup when fix-perms is standalone
Because `--fix-perms` defaults to backup mode (Issue 2), the following unnecessary work happens:
- btrfs snapshot creation
- duplicacy working directory setup
- duplicacy preferences file writing
- Full duplicacy backup execution
- Snapshot cleanup

For a standalone `--fix-perms`, the only things needed are:
1. Parse config to get `DESTINATION`, `LOCAL_OWNER`, `LOCAL_GROUP`
2. Resolve `backupTarget` path
3. Run `permissions.Fix()` on that path
4. Exit

---

## Execution Flow Diagram

### Current (buggy) flow for `--fix-perms homes`:
```
duplicacy-backup --fix-perms homes
                    │
                    ▼
            parseFlags() → f.mode="" → defaults to "backup"
                    │        f.fixPerms=true
                    ▼
            doBackup=true, doPrune=false
                    │
                    ▼
            [All config/validation/setup runs]
                    │
                    ▼
            btrfs.CreateSnapshot()  ← UNEXPECTED: creates snapshot
                    │
                    ▼
            dup.RunBackup()         ← UNEXPECTED: runs full backup
                    │
                    ▼
            permissions.Fix(backupTarget, ...)  ← Correct path, but buried after backup
                    │
                    ▼
            doCleanup() → deletes snapshot
```

### Desired flow for `--fix-perms homes` (standalone):
```
duplicacy-backup --fix-perms homes
                    │
                    ▼
            parseFlags() → f.mode="" stays empty
                    │        f.fixPerms=true
                    ▼
            doBackup=false, doPrune=false
                    │
                    ▼
            [Config parsing + validation only]
                    │
                    ▼
            backupTarget = /volume2/backups/homes  ← Correct destination path
                    │
                    ▼
            permissions.Fix(backupTarget, owner, group)  ← Quick, standalone
                    │
                    ▼
            Exit success
```

### Combined flow: `--fix-perms --backup homes`:
```
duplicacy-backup --backup --fix-perms homes
                    │
                    ▼
            parseFlags() → f.mode="backup", f.fixPerms=true
                    │
                    ▼
            [Normal backup flow]
                    │
                    ▼
            btrfs.CreateSnapshot() → dup.RunBackup()
                    │
                    ▼
            permissions.Fix(backupTarget, ...)  ← Fix perms after backup
                    │
                    ▼
            doCleanup()
```

---

## Proposed Fixes

### Fix 1: Make `--fix-perms` + `--remote` a hard error
```go
if f.fixPerms && f.remoteMode {
    fmt.Fprintln(os.Stderr, "[ERROR] --fix-perms cannot be used with --remote (fix-perms is a local-only operation)")
    return 1
}
```

### Fix 2: Don't default to backup mode when `--fix-perms` is set alone
```go
// Default mode is backup — but not if --fix-perms is the sole operation
if f.mode == "" && !f.fixPerms {
    f.mode = "backup"
}
```
This allows:
- `duplicacy-backup --fix-perms homes` → mode stays empty, only fix-perms runs
- `duplicacy-backup --backup --fix-perms homes` → mode="backup", backup + fix-perms
- `duplicacy-backup homes` → mode defaults to "backup" (unchanged)

### Fix 3: Require at least one operation
After the mode defaulting logic, validate that something will actually run:
```go
if f.mode == "" && !f.fixPerms {
    return nil, fmt.Errorf("no operation specified; use --backup, --prune, --prune-deep, or --fix-perms")
}
```
(This is already implicitly handled by the existing default-to-backup logic for the non-fix-perms case, but a guard is good practice.)

### Fix 4: Skip backup/prune infrastructure when fix-perms is standalone
When `doBackup=false` and `doPrune=false` and `fixPerms=true`:
- Skip btrfs snapshot creation
- Skip duplicacy setup (CreateDirs, WritePreferences, WriteFilters, SetPermissions)
- Skip duplicacy backup/prune execution
- Go straight to `permissions.Fix()`

This is mostly already correct since these blocks are guarded by `doBackup`/`doPrune`, **except** for:
- `dup.CreateDirs()` (line 441) — always runs
- `dup.WritePreferences()` (line 448) — always runs
- `dup.SetPermissions()` (line 464) — always runs
- `btrfs.CheckVolume(log, snapshotSource, ...)` — guarded by `doBackup` ✅

We need to guard the duplicacy setup block with `doBackup || doPrune`.

### Fix 5: Add operation mode logging for fix-perms
```go
if f.fixPerms && !doBackup && !doPrune {
    log.PrintLine("Operation Mode", "Fix permissions only")
} else if f.fixPerms {
    // Combined with another mode
    log.PrintLine("Fix Perms", "yes (after backup/prune)")
}
```

### Fix 6: Add timing to fix-perms execution
```go
if !f.remoteMode && f.fixPerms {
    start := time.Now()
    log.Info("Fixing permissions on %s (owner=%s, group=%s)", backupTarget, cfg.LocalOwner, cfg.LocalGroup)
    if err := permissions.Fix(log, backupTarget, cfg.LocalOwner, cfg.LocalGroup, f.dryRun); err != nil {
        ...
    }
    log.Info("Permission fix completed in %s", time.Since(start).Round(time.Millisecond))
}
```

---

## Summary of Changes Needed

| # | Issue | Severity | Fix |
|---|-------|----------|-----|
| 1 | `--fix-perms` + `--remote` only warns | Medium | Hard error (exit 1) |
| 2 | `--fix-perms` alone triggers a backup | **High** | Don't default to backup when fix-perms set |
| 3 | No operation mode output for fix-perms | Low | Add "Fix permissions only" log line |
| 4 | Unnecessary duplicacy setup for standalone fix-perms | Medium | Guard setup blocks with `doBackup \|\| doPrune` |
| 5 | No timing output | Low | Add start/complete timing |

### What is NOT an issue (corrected from previous analysis)
- **Target path (`backupTarget`) is CORRECT** — it correctly points to the local backup destination
- **`permissions.Fix()` logic is correct** — `chown -R` + `chmod 770/660` is appropriate for backup storage
- **Guard `!f.remoteMode`** is correct — properly prevents running on remote mode

### Files to Modify
1. `cmd/duplicacy-backup/main.go` — All 5 fixes
2. `README.md` — Update `--fix-perms` documentation
3. `CHANGELOG.md` — Document the fixes
