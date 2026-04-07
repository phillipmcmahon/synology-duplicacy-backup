// duplicacy-backup is a compiled replacement for the duplicacy-backup.sh script.
// It performs Duplicacy backups on Synology NAS using btrfs snapshots, with support
// for local and remote backup modes, safe pruning with threshold guards, and
// directory-based concurrency locking.
//
// Command model:
//
//      default                           -> backup only
//      --backup                          -> backup only
//      --prune                           -> safe, threshold-guarded policy prune only
//      --prune --force-prune             -> safe policy prune, threshold override allowed
//      --prune-deep --force-prune        -> maintenance mode: policy prune + exhaustive exclusive prune
//      --fix-perms                       -> normalise local repository ownership/permissions
package main

import (
        "fmt"
        "os"
        "os/exec"
        "os/signal"
        "path/filepath"
        "strings"
        "syscall"
        "time"

        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/permissions"
        "github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

const (
        rootVolume = "/volume1"
        logDir     = "/var/log"
        lockParent = "/var/lock"
        scriptName = "duplicacy-backup"
)

// flags holds all CLI flags parsed from arguments.
type flags struct {
        mode       string // "backup", "prune", "prune-deep"
        fixPerms   bool
        forcePrune bool
        remoteMode bool
        dryRun     bool
        configDir  string // override config directory
        secretsDir string // override secrets directory
        source     string // positional arg: source directory name
}

func main() {
        os.Exit(run())
}

func run() int {
        // Check for --help before any privilege/dependency checks so that
        // help text is always accessible regardless of environment.
        for _, arg := range os.Args[1:] {
                if arg == "--help" {
                        printUsage()
                        return 0
                }
        }

        // Must be root
        if os.Geteuid() != 0 {
                fmt.Fprintln(os.Stderr, "[ERROR] Must be run as root.")
                return 1
        }

        // Check required commands
        for _, cmd := range []string{"btrfs", "duplicacy"} {
                if _, err := exec.LookPath(cmd); err != nil {
                        fmt.Fprintf(os.Stderr, "[ERROR] Required command '%s' not found\n", cmd)
                        return 1
                }
        }

        // Detect colour support
        enableColour := isTerminal(os.Stderr)

        // Parse CLI flags
        f, err := parseFlags(os.Args[1:])
        if err != nil {
                fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
                return 1
        }

        // Derive modes
        doBackup := f.mode == "backup"
        doPrune := f.mode == "prune" || f.mode == "prune-deep"
        deepPruneMode := f.mode == "prune-deep"

        // Validate flag combinations
        if deepPruneMode && !f.forcePrune {
                fmt.Fprintln(os.Stderr, "[ERROR] --prune-deep requires --force-prune")
                return 1
        }

        if f.forcePrune && !doPrune {
                fmt.Fprintln(os.Stderr, "[WARNING] --force-prune has no effect unless used with --prune or --prune-deep")
        }

        if f.fixPerms && f.remoteMode {
                fmt.Fprintln(os.Stderr, "[WARNING] --fix-perms has no effect with --remote")
        }

        // Timestamps
        runTimestamp := time.Now().Format("20060102-150405")

        // Set up logger
        log, err := logger.New(logDir, scriptName, enableColour)
        if err != nil {
                fmt.Fprintf(os.Stderr, "[ERROR] Failed to initialise logger: %v\n", err)
                return 1
        }
        defer log.Close()

        // Derive paths
        backupLabel := f.source
        snapshotSource := filepath.Join(rootVolume, backupLabel)
        snapshotTarget := filepath.Join(rootVolume, fmt.Sprintf("%s-%s", backupLabel, runTimestamp))
        workRoot := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s-%s-%d", scriptName, backupLabel, runTimestamp, os.Getpid()))

        var repositoryPath string
        if doBackup {
                repositoryPath = snapshotTarget
        } else {
                repositoryPath = snapshotSource
        }

        // Acquire lock
        lk := lock.New(lockParent, backupLabel)

        // Set up cleanup and signal handling
        exitCode := 0
        cleanedUp := false

        doCleanup := func(code int) {
                if cleanedUp {
                        return
                }
                cleanedUp = true

                log.Info("Starting cleanup...")

                if doBackup {
                        if _, err := os.Stat(snapshotTarget); err == nil {
                                log.Info("Deleting snapshot subvolume... %s", snapshotTarget)
                                if delErr := btrfs.DeleteSnapshot(log, snapshotTarget, f.dryRun); delErr != nil {
                                        log.Warn("Failed to delete subvolume %s: %v", snapshotTarget, delErr)
                                        // Also try rm -rf as fallback
                                        os.RemoveAll(snapshotTarget)
                                }
                        }
                }

                if _, err := os.Stat(workRoot); err == nil {
                        log.Info("Removing duplicacy work directory... %s", workRoot)
                        if f.dryRun {
                                log.DryRun("rm -rf %s", workRoot)
                        } else {
                                os.RemoveAll(workRoot)
                        }
                }

                lk.Release()

                status := "SUCCESS"
                if code != 0 {
                        status = "FAILED"
                }

                log.PrintSeparator()
                log.Info("Backup script completed:")
                log.PrintLine("Result", log.FormatResult(status))
                log.PrintLine("Code", fmt.Sprintf("%d", code))
                log.PrintLine("Timestamp", time.Now().Format("2006-01-02 15:04:05"))
                log.PrintSeparator()
        }

        defer doCleanup(exitCode)

        // Handle signals
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
        go func() {
                sig := <-sigChan
                log.Warn("Received signal: %v - initiating cleanup...", sig)
                doCleanup(1)
                os.Exit(1)
        }()

        // Acquire the lock
        if err := lk.Acquire(); err != nil {
                log.Error("Lock acquisition failed: %v", err)
                exitCode = 1
                return exitCode
        }

        // Header
        log.PrintSeparator()
        log.Info("Backup Script Started - %s", time.Now().Format("2006-01-02 15:04:05"))
        log.PrintLine("Script", scriptName)
        log.PrintLine("PID", fmt.Sprintf("%d", os.Getpid()))
        log.PrintLine("Lock Path", lk.Path)
        log.PrintSeparator()

        // Resolve config directory: flag > env > default (executable dir + .config)
        configDir := resolveDir(f.configDir, "DUPLICACY_BACKUP_CONFIG_DIR", executableConfigDir())
        configFile := filepath.Join(configDir, fmt.Sprintf("%s-backup.conf", backupLabel))

        if _, err := os.Stat(configFile); os.IsNotExist(err) {
                log.Error("Configuration file %s not found", configFile)
                exitCode = 1
                return exitCode
        }

        // Parse config
        cfg := config.NewDefaults()

        targetSection := "local"
        if f.remoteMode {
                targetSection = "remote"
        }

        values, err := config.ParseFile(configFile, targetSection)
        if err != nil {
                log.Error("Config parse error: %v", err)
                exitCode = 1
                return exitCode
        }
        cfg.Apply(values)

        log.Info("Config file %s parsed for [common] + [%s]", configFile, targetSection)

        // Clean up old logs
        log.CleanupOldLogs(cfg.LogRetentionDays, f.dryRun)

        // Validate config
        if err := cfg.ValidateRequired(doBackup, doPrune); err != nil {
                log.Error("Config file: %s", configFile)
                log.Error("%v", err)
                exitCode = 1
                return exitCode
        }

        if err := cfg.ValidateThresholds(); err != nil {
                log.Error("%v", err)
                exitCode = 1
                return exitCode
        }

        if err := cfg.ValidateOwnerGroup(); err != nil {
                log.Error("%v", err)
                exitCode = 1
                return exitCode
        }

        cfg.BuildPruneArgs()

        if doBackup {
                if err := cfg.ValidateThreads(); err != nil {
                        log.Error("%v", err)
                        exitCode = 1
                        return exitCode
                }
        }

        // Check btrfs volumes
        if err := btrfs.CheckVolume(log, rootVolume, f.dryRun); err != nil {
                log.Error("%v", err)
                exitCode = 1
                return exitCode
        }

        if doBackup {
                if err := btrfs.CheckVolume(log, snapshotSource, f.dryRun); err != nil {
                        log.Error("%v", err)
                        exitCode = 1
                        return exitCode
                }
        }

        // Load secrets for remote mode
        backupTarget := joinDestination(cfg.Destination, backupLabel)
        var sec *secrets.Secrets

        // Resolve secrets directory: flag > env > default
        secretsDir := resolveDir(f.secretsDir, "DUPLICACY_BACKUP_SECRETS_DIR", config.DefaultSecretsDir)

        if f.remoteMode {
                secretsFile := secrets.GetSecretsFilePath(secretsDir, config.DefaultSecretsPrefix, backupLabel)
                sec, err = secrets.LoadSecretsFile(secretsFile)
                if err != nil {
                        log.Error("%v", err)
                        exitCode = 1
                        return exitCode
                }
                if err := sec.Validate(); err != nil {
                        log.Error("%v", err)
                        exitCode = 1
                        return exitCode
                }
                log.Info("Secrets loaded from %s", secretsFile)
        }

        // Print config summary
        modeStr := "LOCAL"
        if f.remoteMode {
                modeStr = "REMOTE"
        }

        log.Info("Configuration Summary:")
        log.PrintLine("Config File", configFile)
        log.PrintLine("Backup Label", backupLabel)
        log.PrintLine("Mode", modeStr)
        log.PrintLine("Source", snapshotSource)
        log.PrintLine("Repository", repositoryPath)
        log.PrintLine("Work Dir", filepath.Join(workRoot, "duplicacy"))
        log.PrintLine("Destination", backupTarget)
        if cfg.Threads > 0 {
                log.PrintLine("Threads", fmt.Sprintf("%d", cfg.Threads))
        } else {
                log.PrintLine("Threads", "<n/a>")
        }
        if cfg.Filter != "" {
                log.PrintLine("Filter", cfg.Filter)
        } else {
                log.PrintLine("Filter", "<none>")
        }
        if cfg.Prune != "" {
                log.PrintLine("Prune Options", cfg.Prune)
        } else {
                log.PrintLine("Prune Options", "<none>")
        }
        log.PrintLine("Local Owner", cfg.LocalOwner)
        log.PrintLine("Local Group", cfg.LocalGroup)
        log.PrintLine("Log Retention", fmt.Sprintf("%d", cfg.LogRetentionDays))
        log.PrintLine("Dry Run", fmt.Sprintf("%t", f.dryRun))
        log.PrintLine("Force Prune", fmt.Sprintf("%t", f.forcePrune))
        log.PrintLine("Fix Perms", fmt.Sprintf("%t", f.fixPerms))
        log.PrintLine("Prune Max %", fmt.Sprintf("%d", cfg.SafePruneMaxDeletePercent))
        log.PrintLine("Prune Max Count", fmt.Sprintf("%d", cfg.SafePruneMaxDeleteCount))
        log.PrintLine("Prune Min Total %", fmt.Sprintf("%d", cfg.SafePruneMinTotalForPercent))

        if f.remoteMode && sec != nil {
                log.PrintLine("Secrets File", secrets.GetSecretsFilePath(secretsDir, config.DefaultSecretsPrefix, backupLabel))
                log.PrintLine("STORJ S3 ID", sec.MaskedID())
                log.PrintLine("STORJ S3 Secret", sec.MaskedSecret())
        }

        // Print operation mode
        if doBackup {
                log.PrintLine("Operation Mode", "Backup only")
        } else if doPrune && deepPruneMode {
                log.PrintLine("Operation Mode", "Prune deep")
        } else if doPrune {
                log.PrintLine("Operation Mode", "Prune safe")
        }

        // Create btrfs snapshot for backup
        if doBackup {
                if err := btrfs.CreateSnapshot(log, snapshotSource, snapshotTarget, f.dryRun); err != nil {
                        log.Error("Failed to create snapshot: %v", err)
                        exitCode = 1
                        return exitCode
                }
        }

        // Set up duplicacy working environment
        dup := duplicacy.NewSetup(workRoot, repositoryPath, backupTarget, log, f.dryRun)

        if err := dup.CreateDirs(); err != nil {
                log.Error("%v", err)
                exitCode = 1
                return exitCode
        }

        // Write preferences
        if err := dup.WritePreferences(sec); err != nil {
                log.Error("Failed to write preferences: %v", err)
                exitCode = 1
                return exitCode
        }

        // Write filters for backup mode
        if doBackup && cfg.Filter != "" {
                if err := dup.WriteFilters(cfg.Filter); err != nil {
                        log.Error("%v", err)
                        exitCode = 1
                        return exitCode
                }
        }

        // Set permissions on work directory
        if err := dup.SetPermissions(); err != nil {
                log.Error("%v", err)
                exitCode = 1
                return exitCode
        }

        log.Info("Changing to directory: %s", dup.DuplicacyRoot)

        // Run backup
        if doBackup {
                if err := dup.RunBackup(cfg.Threads); err != nil {
                        log.Error("Backup failed: %v", err)
                        exitCode = 1
                        return exitCode
                }
        }

        // Run prune
        if doPrune {
                if err := dup.ValidateRepo(); err != nil {
                        log.Error("Cannot perform prune operation - repository not ready: %v", err)
                        exitCode = 1
                        return exitCode
                }

                preview, err := dup.SafePrunePreview(cfg.PruneArgs, cfg.SafePruneMinTotalForPercent)
                if err != nil {
                        log.Error("Safe prune preview failed: %v", err)
                        exitCode = 1
                        return exitCode
                }

                // Display preview
                log.PrintLine("Preview Deletes", fmt.Sprintf("%d", preview.DeleteCount))
                log.PrintLine("Preview Total Revs", fmt.Sprintf("%d", preview.TotalRevisions))
                if preview.PercentEnforced {
                        log.PrintLine("Preview Delete %", fmt.Sprintf("%d", preview.DeletePercent))
                } else {
                        log.PrintLine("Preview Delete %", fmt.Sprintf("<not enforced; total revisions unavailable or below %d>", cfg.SafePruneMinTotalForPercent))
                }

                // Check thresholds
                blocked := false
                if preview.DeleteCount > cfg.SafePruneMaxDeleteCount {
                        log.Error("Safe prune preview exceeds delete count threshold: %d > %d", preview.DeleteCount, cfg.SafePruneMaxDeleteCount)
                        blocked = true
                }
                if preview.PercentEnforced && preview.DeletePercent > cfg.SafePruneMaxDeletePercent {
                        log.Error("Safe prune preview exceeds delete percentage threshold: %d%% > %d%%", preview.DeletePercent, cfg.SafePruneMaxDeletePercent)
                        blocked = true
                }

                if blocked {
                        if f.forcePrune {
                                log.Warn("Proceeding despite safe prune threshold breach because --force-prune was supplied")
                        } else {
                                log.Error("Refusing to continue because safe prune thresholds were exceeded")
                                exitCode = 1
                                return exitCode
                        }
                }

                if err := dup.RunPrune(cfg.PruneArgs); err != nil {
                        log.Error("Policy prune failed: %v", err)
                        exitCode = 1
                        return exitCode
                }

                if deepPruneMode {
                        if err := dup.RunDeepPrune(); err != nil {
                                log.Error("Deep prune failed: %v", err)
                                exitCode = 1
                                return exitCode
                        }
                }
        }

        // Fix permissions for local mode
        if !f.remoteMode && f.fixPerms {
                if err := permissions.Fix(log, backupTarget, cfg.LocalOwner, cfg.LocalGroup, f.dryRun); err != nil {
                        log.Error("%v", err)
                        exitCode = 1
                        return exitCode
                }
        }

        log.Info("All operations completed")
        return 0
}

func parseFlags(args []string) (*flags, error) {
        f := &flags{mode: ""}
        var positional []string

        for i := 0; i < len(args); i++ {
                switch args[i] {
                case "--backup", "--prune", "--prune-deep":
                        if f.mode != "" {
                                return nil, fmt.Errorf("only one mode may be specified: --backup, --prune, or --prune-deep")
                        }
                        f.mode = strings.TrimPrefix(args[i], "--")
                case "--fix-perms":
                        f.fixPerms = true
                case "--force-prune":
                        f.forcePrune = true
                case "--remote":
                        f.remoteMode = true
                case "--dry-run":
                        f.dryRun = true
                case "--config-dir":
                        if i+1 >= len(args) {
                                return nil, fmt.Errorf("--config-dir requires a value")
                        }
                        i++
                        f.configDir = args[i]
                case "--secrets-dir":
                        if i+1 >= len(args) {
                                return nil, fmt.Errorf("--secrets-dir requires a value")
                        }
                        i++
                        f.secretsDir = args[i]
                case "--help":
                        printUsage()
                        os.Exit(0)
                default:
                        if strings.HasPrefix(args[i], "-") {
                                return nil, fmt.Errorf("unknown option %s", args[i])
                        }
                        positional = append(positional, args[i])
                }
        }

        // Default mode is backup
        if f.mode == "" {
                f.mode = "backup"
        }

        if len(positional) < 1 {
                return nil, fmt.Errorf("source directory required")
        }

        f.source = positional[0]
        return f, nil
}

func printUsage() {
        defaultCfgDir := executableConfigDir()
        fmt.Printf(`Usage: %s [OPTIONS] <source>

DEFAULT BEHAVIOUR:
    No mode specified = backup only

MODES:
    --backup                 Perform backup only
    --prune                  Perform safe, threshold-guarded policy prune only
    --prune-deep             Perform maintenance prune mode (requires --force-prune):
                             policy prune + exhaustive exclusive prune

MODIFIERS:
    --fix-perms              Normalise local repository ownership and permissions
    --force-prune            Override safe prune thresholds, or authorise --prune-deep
    --remote                 Perform operation against remote target config
    --dry-run                Simulate actions without making changes
    --config-dir <path>      Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>     Override secrets directory (default: %s)
    --help                   Show this help message

ENVIRONMENT VARIABLES:
    DUPLICACY_BACKUP_CONFIG_DIR   Override config directory (--config-dir takes precedence)
    DUPLICACY_BACKUP_SECRETS_DIR  Override secrets directory (--secrets-dir takes precedence)

SAFE PRUNE THRESHOLDS:
    Max delete percent       : %d%% (default %d%%)
    Max delete count         : %d (default %d)
    Min revisions for %% check: %d (default %d)

CONFIG FILE LOCATION:
    <binary-dir>/.config/<source>-backup.conf
    Currently resolves to: %s/<source>-backup.conf
    Override with --config-dir or DUPLICACY_BACKUP_CONFIG_DIR

CONFIG KEYS:
    DESTINATION, FILTER, LOCAL_OWNER, LOCAL_GROUP, LOG_RETENTION_DAYS,
    PRUNE, THREADS, SAFE_PRUNE_MAX_DELETE_COUNT, SAFE_PRUNE_MAX_DELETE_PERCENT,
    SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT

REMOTE SECRETS:
    Strict mode: remote credentials are loaded only from:
      %s/%s-<label>.env
    Override directory with --secrets-dir or DUPLICACY_BACKUP_SECRETS_DIR

ARGUMENTS:
    source                   Source directory name under %s

EXAMPLES:
    %s homes
    %s --backup homes
    %s --prune homes
    %s --prune --force-prune homes
    %s --prune-deep --force-prune homes
    %s --fix-perms homes
    %s --remote homes
    %s --config-dir /opt/etc homes
    %s --secrets-dir /opt/secrets --remote homes
`, scriptName,
                config.DefaultSecretsDir,
                config.DefaultSafePruneMaxDeletePercent, config.DefaultSafePruneMaxDeletePercent,
                config.DefaultSafePruneMaxDeleteCount, config.DefaultSafePruneMaxDeleteCount,
                config.DefaultSafePruneMinTotalForPercent, config.DefaultSafePruneMinTotalForPercent,
                defaultCfgDir,
                config.DefaultSecretsDir, config.DefaultSecretsPrefix,
                rootVolume,
                scriptName, scriptName, scriptName, scriptName, scriptName, scriptName, scriptName,
                scriptName, scriptName,
        )
}

// executableConfigDir returns the directory containing the running binary plus
// "/.config".  This lets config files travel alongside the binary, which is the
// typical Synology deployment layout.  If the executable path cannot be
// determined (e.g. in test harnesses) it falls back to "./.config".
func executableConfigDir() string {
        exe, err := os.Executable()
        if err != nil {
                return filepath.Join(".", ".config")
        }
        // Resolve symlinks so the real binary location is used.
        exe, err = filepath.EvalSymlinks(exe)
        if err != nil {
                return filepath.Join(".", ".config")
        }
        return filepath.Join(filepath.Dir(exe), ".config")
}

// resolveDir returns the directory path from flag, env var, or default (in that priority order).
func resolveDir(flagValue, envVar, defaultDir string) string {
        if flagValue != "" {
                return flagValue
        }
        if v := os.Getenv(envVar); v != "" {
                return v
        }
        return defaultDir
}

// joinDestination appends a label to a destination path.
// For URL-style destinations (containing "://"), it preserves the scheme
// separator which filepath.Join would incorrectly collapse (e.g. s3:// → s3:/).
func joinDestination(destination, label string) string {
        if idx := strings.Index(destination, "://"); idx >= 0 {
                // Split into scheme+authority and path portion after "://"
                scheme := destination[:idx+3] // e.g. "s3://"
                rest := destination[idx+3:]   // e.g. "EU@gateway.storjshare.io/bucket"
                rest = strings.TrimRight(rest, "/")
                return scheme + rest + "/" + label
        }
        return filepath.Join(destination, label)
}

func isTerminal(f *os.File) bool {
        fi, err := f.Stat()
        if err != nil {
                return false
        }
        return (fi.Mode() & os.ModeCharDevice) != 0
}
