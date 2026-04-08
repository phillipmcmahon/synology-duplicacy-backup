// duplicacy-backup is a compiled replacement for the duplicacy-backup.sh script.
// It performs Duplicacy backups on Synology NAS using btrfs snapshots, with support
// for local and remote backup modes, safe pruning with threshold guards, and
// directory-based concurrency locking.
//
// Command model:
//
//	default                           -> backup only
//	--backup                          -> backup only
//	--prune                           -> safe, threshold-guarded policy prune only
//	--prune --force-prune             -> safe policy prune, threshold override allowed
//	--prune-deep --force-prune        -> maintenance mode: policy prune + exhaustive exclusive prune
//	--fix-perms                       -> normalise local repository ownership/permissions
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
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

// version and buildTime are set at build time via -ldflags.
// See Makefile for the injection pattern:
//
//	go build -ldflags "-X main.version=... -X main.buildTime=..."
var (
	version   = "1.7.3"
	buildTime = "unknown"
)

const (
	rootVolume = "/volume1"
	logDir     = "/var/log"
	lockParent = "/var/lock"
	scriptName = "duplicacy-backup"
)

// labelPattern restricts backup labels to safe characters only.
// This prevents path traversal attacks since labels are interpolated into
// filesystem paths (config files, secrets, lock dirs, snapshots).
var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// validateLabel checks that label contains only safe characters and cannot
// be used for path traversal. Labels are used in filesystem paths for config,
// secrets, locks, and snapshots, so they must not contain path separators,
// parent-directory references, or other dangerous characters.
func validateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("label must not be empty")
	}
	if strings.Contains(label, "/") || strings.Contains(label, "\\") || strings.Contains(label, "..") {
		return fmt.Errorf("label %q contains path traversal characters (/, \\, or ..); "+
			"only alphanumeric characters, hyphens, and underscores are allowed", label)
	}
	if !labelPattern.MatchString(label) {
		return fmt.Errorf("label %q contains invalid characters; "+
			"only alphanumeric characters (a-z, A-Z, 0-9), hyphens (-), and underscores (_) are allowed, "+
			"and must start with an alphanumeric character", label)
	}
	return nil
}

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
	// Check for --help and --version before any privilege/dependency checks
	// so that help/version text is always accessible regardless of environment.
	for _, arg := range os.Args[1:] {
		if arg == "--help" {
			printUsage()
			return 0
		}
		if arg == "--version" || arg == "-v" {
			fmt.Printf("%s %s (built %s)\n", scriptName, version, buildTime)
			return 0
		}
	}

	// Detect colour support (auto-detect TTY on stderr)
	enableColour := logger.IsTerminal(os.Stderr)

	// Initialise logger early so that ALL error/warning messages benefit
	// from consistent formatting (timestamps, colour, log-file capture).
	// Only the logger-init-failure message below must fall back to raw stderr.
	log, err := logger.New(logDir, scriptName, enableColour)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to initialise logger: %v\n", err)
		return 1
	}
	defer log.Close()

	// Must be root
	if os.Geteuid() != 0 {
		log.Error("Must be run as root.")
		return 1
	}

	// Parse CLI flags
	f, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Error("%v", err)
		fmt.Fprintln(os.Stderr)
		printUsage()
		return 1
	}

	// Validate label before any filesystem operations (security: prevent path traversal)
	if err := validateLabel(f.source); err != nil {
		log.Error("Invalid source label: %v", err)
		return 1
	}

	// Derive modes
	doBackup := f.mode == "backup"
	doPrune := f.mode == "prune" || f.mode == "prune-deep"
	deepPruneMode := f.mode == "prune-deep"
	fixPermsOnly := f.fixPerms && !doBackup && !doPrune

	// Check duplicacy binary – only needed for backup or prune operations.
	// Skip for standalone --fix-perms which only calls chown/chmod.
	if doBackup || doPrune {
		if _, err := exec.LookPath("duplicacy"); err != nil {
			log.Error("Required command 'duplicacy' not found")
			return 1
		}
	}

	// Check btrfs command – only needed for backup (snapshot create/delete)
	if doBackup {
		if _, err := exec.LookPath("btrfs"); err != nil {
			log.Error("Required command 'btrfs' not found (needed for backup snapshots)")
			return 1
		}
	}

	// Validate flag combinations
	if deepPruneMode && !f.forcePrune {
		log.Error("--prune-deep requires --force-prune")
		return 1
	}

	// Timestamps
	runTimestamp := time.Now().Format("20060102-150405")

	if f.forcePrune && !doPrune {
		log.Error("--force-prune requires --prune or --prune-deep")
		return 1
	}

	if f.fixPerms && f.remoteMode {
		log.Error("--fix-perms is only valid for local backups; cannot be used with --remote")
		return 1
	}

	if f.mode == "" {
		if f.fixPerms {
			log.Info("No primary mode specified: using fix-perms only")
		} else {
			log.Info("No mode specified: defaulting to backup only")
		}
	}

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
				}
			}

			if _, err := os.Stat(snapshotTarget); err == nil {
				log.Info("Removing snapshot directory... %s", snapshotTarget)
				if f.dryRun {
					log.DryRun("rm -rf %s", snapshotTarget)
				} else {
					if rmErr := os.RemoveAll(snapshotTarget); rmErr != nil {
						log.Warn("Failed to remove snapshot directory %s", snapshotTarget)
					}
				}
			}
		}

		if _, err := os.Stat(workRoot); err == nil {
			log.Info("Removing duplicacy work directory... %s", workRoot)
			if f.dryRun {
				log.DryRun("rm -rf %s", workRoot)
			} else {
				if rmErr := os.RemoveAll(workRoot); rmErr != nil {
					log.Warn("Failed to remove work directory %s: %v", workRoot, rmErr)
				}
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

	// Use a closure so that doCleanup receives the final value of exitCode,
	// not the value at the time defer is evaluated (which would always be 0).
	defer func() { doCleanup(exitCode) }()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Warn("Received signal: %v  initiating cleanup...", sig)
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
	log.Info("Backup script started - %s", time.Now().Format("2006-01-02 15:04:05"))
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
		log.Error("%v", err)
		exitCode = 1
		return exitCode
	}
	if err := cfg.Apply(values); err != nil {
		log.Error("%v", err)
		exitCode = 1
		return exitCode
	}

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

	// LOCAL_OWNER and LOCAL_GROUP are only needed when --fix-perms will
	// actually change file ownership.  Skip the (potentially expensive)
	// user/group look-ups for plain backup or prune runs.
	if f.fixPerms {
		if err := cfg.ValidateOwnerGroup(); err != nil {
			log.Error("%v", err)
			exitCode = 1
			return exitCode
		}
	}

	cfg.BuildPruneArgs()

	if doBackup {
		if err := cfg.ValidateThreads(); err != nil {
			log.Error("%v", err)
			exitCode = 1
			return exitCode
		}
	}

	// Check btrfs volumes – only needed for backup (snapshot creation)
	if doBackup {
		if err := btrfs.CheckVolume(log, rootVolume, f.dryRun); err != nil {
			log.Error("%v", err)
			exitCode = 1
			return exitCode
		}

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

	// Determine operation mode string (printed early in the summary)
	var opMode string
	if fixPermsOnly {
		opMode = "Fix permissions only"
	} else if doBackup && f.fixPerms {
		opMode = "Backup + fix permissions"
	} else if doBackup {
		opMode = "Backup only"
	} else if doPrune && deepPruneMode && f.fixPerms {
		opMode = "Prune deep + fix permissions"
	} else if doPrune && deepPruneMode {
		opMode = "Prune deep"
	} else if doPrune && f.fixPerms {
		opMode = "Prune safe + fix permissions"
	} else if doPrune {
		opMode = "Prune safe"
	}

	log.Info("Configuration Summary:")
	log.PrintLine("Operation Mode", opMode)

	if fixPermsOnly {
		// Match the bash script's standalone fix-perms summary layout.
		log.PrintLine("Destination", backupTarget)
		log.PrintLine("Local Owner", cfg.LocalOwner)
		log.PrintLine("Local Group", cfg.LocalGroup)
		log.PrintLine("Dry Run", fmt.Sprintf("%t", f.dryRun))
	} else {
		// Match the bash script's full summary field ordering and labels.
		log.PrintLine("Config File", configFile)
		log.PrintLine("Backup Label", backupLabel)
		log.PrintLine("Mode", modeStr)
		log.PrintLine("Source", snapshotSource)
		log.PrintLine("Snapshot", repositoryPath)
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
		// Only show Local Owner/Group when --fix-perms is active
		// (these fields are not relevant for plain backup or prune).
		if f.fixPerms {
			log.PrintLine("Local Owner", cfg.LocalOwner)
			log.PrintLine("Local Group", cfg.LocalGroup)
		}
		log.PrintLine("Log Retention", fmt.Sprintf("%d", cfg.LogRetentionDays))
		log.PrintLine("Dry Run", fmt.Sprintf("%t", f.dryRun))
		log.PrintLine("Force Prune", fmt.Sprintf("%t", f.forcePrune))
		log.PrintLine("Fix Perms", fmt.Sprintf("%t", f.fixPerms))
		log.PrintLine("Prune Max %", fmt.Sprintf("%d", cfg.SafePruneMaxDeletePercent))
		log.PrintLine("Prune Max Count", fmt.Sprintf("%d", cfg.SafePruneMaxDeleteCount))
		log.PrintLine("Prune Min Total Revs", fmt.Sprintf("%d", cfg.SafePruneMinTotalForPercent))

		if f.remoteMode && sec != nil {
			log.PrintLine("Secrets Dir", secretsDir)
			log.PrintLine("Secrets File", secrets.GetSecretsFilePath(secretsDir, config.DefaultSecretsPrefix, backupLabel))
			log.PrintLine("STORJ S3 ID", sec.MaskedID())
			log.PrintLine("STORJ S3 Secret", sec.MaskedSecret())
		}
	}

	// BTRFS snapshot and Duplicacy setup – only needed for backup/prune
	if doBackup || doPrune {
		// Create btrfs snapshot for backup
		if doBackup {
			if err := btrfs.CreateSnapshot(log, snapshotSource, snapshotTarget, f.dryRun); err != nil {
				log.Error("Failed to create snapshot")
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
				log.Error("Backup failed")
				exitCode = 1
				return exitCode
			}
		}

		// Run prune
		if doPrune {
			if err := dup.ValidateRepo(); err != nil {
				log.Error("Cannot perform prune operation - repository not ready")
				exitCode = 1
				return exitCode
			}

			preview, err := dup.SafePrunePreview(cfg.PruneArgs, cfg.SafePruneMinTotalForPercent)
			if err != nil {
				log.Error("Safe prune preview failed")
				exitCode = 1
				return exitCode
			}

			// Fail-closed: if revision count failed, block unless --force-prune
			if preview.RevisionCountFailed {
				if f.forcePrune {
					log.Warn("Revision count failed; proceeding because --force-prune was supplied (percentage threshold not enforced)")
				} else {
					log.Error("Revision count is required for safe prune but failed; use --force-prune to override")
					exitCode = 1
					return exitCode
				}
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
			if preview.ExceedsPercent(cfg.SafePruneMaxDeletePercent) {
				log.Error("Safe prune preview exceeds delete percentage threshold (%d of %d revisions > %d%%)", preview.DeleteCount, preview.TotalRevisions, cfg.SafePruneMaxDeletePercent)
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
				log.Error("Policy prune failed")
				exitCode = 1
				return exitCode
			}

			if deepPruneMode {
				if err := dup.RunDeepPrune(); err != nil {
					log.Error("Deep prune failed")
					exitCode = 1
					return exitCode
				}
			}
		}
	} // end doBackup || doPrune

	// Fix permissions for local mode (standalone or combined with backup/prune)
	if f.fixPerms {
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
		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("unknown option %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	// Default mode is backup – but only when --fix-perms is not the sole
	// operation.  Running `--fix-perms homes` should NOT trigger a full backup.
	if f.mode == "" && !f.fixPerms {
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
    --version, -v            Show version and build information
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
