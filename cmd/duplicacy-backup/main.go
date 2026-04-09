// duplicacy-backup is a compiled replacement for the duplicacy-backup.sh script.
// It performs Duplicacy backups on Synology NAS using btrfs snapshots, with support
// for local and remote backup modes, safe pruning with threshold guards, and
// directory-based concurrency locking.
//
// # Architecture
//
// The program follows a coordinator pattern centred on the [app] struct.
// The top-level [run] function creates an *app via [newApp], then calls
// a series of clearly-bounded methods in sequence:
//
//	acquireLock → loadConfig → loadSecrets → printHeader → printSummary → execute → cleanup
//
// Each method has a single concern and logs + returns errors to the caller.
// The caller (run) checks the error and sets the exit code.  This makes
// the control flow readable in one screen and each phase independently testable.
//
// # Output ownership (Phase 3)
//
// Internal packages (btrfs, duplicacy, config, secrets, permissions, lock) do
// work and return structured errors from the [errors] package.  They never log
// directly.  The coordinator is the sole owner of all operator-facing messages:
// it wraps each operation with high-level phase messages, pipes Duplicacy's
// native output through the logger, and formats structured errors into
// consistent, human-readable output.
//
// Free functions ([parseFlags], [validateLabel], [resolveDir], [joinDestination],
// [executableConfigDir], [printUsage]) remain package-level because they are
// pure or side-effect-free and do not need access to app state.
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
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
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
	version   = "1.8.0"
	buildTime = "unknown"
)

const (
	rootVolume = "/volume1"
	lockParent = "/var/lock"
	scriptName = "duplicacy-backup"
)

// logDir is the directory where log files are written.
// It defaults to /var/log for production use on Synology NAS devices.
// Tests override this variable to use a temporary directory so that
// the test suite does not require root/elevated permissions.
var logDir = "/var/log"

// labelPattern restricts backup labels to safe characters only.
// This prevents path traversal attacks since labels are interpolated into
// filesystem paths (config files, secrets, lock dirs, snapshots).
var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ---------------------------------------------------------------------------
// app – the coordinator struct that holds all state for a single run.
// ---------------------------------------------------------------------------

// app is the central coordinator for a single backup/prune/fix-perms run.
// It is created by [newApp] from CLI arguments and holds every piece of
// derived state so that each phase method can operate without parameters.
//
// The coordinator owns all operator-facing output.  Internal packages do
// work and return structured errors; the coordinator formats and logs them.
type app struct {
	log   *logger.Logger
	flags *flags

	// Mode booleans derived from flags.mode during construction.
	doBackup      bool
	doPrune       bool
	deepPruneMode bool
	fixPermsOnly  bool

	// Identifiers and paths derived from the source label.
	backupLabel    string
	runTimestamp   string
	snapshotSource string
	snapshotTarget string
	workRoot       string
	repositoryPath string
	backupTarget   string

	// Configuration and secrets file paths.
	configDir  string
	configFile string
	secretsDir string

	// Loaded configuration, secrets, lock, duplicacy setup, and command runner.
	cfg    *config.Config
	sec    *secrets.Secrets
	lk     *lock.Lock
	dup    *duplicacy.Setup
	runner execpkg.Runner // Shared command runner for all external process execution

	// Exit bookkeeping.
	exitCode     int
	cleanedUp    bool
	lockAcquired bool
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

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	os.Exit(run())
}

// exitHandled is the exit code returned by sub-initialisers (e.g.
// [parseAppFlags]) when the request has been fully handled and the
// process should exit cleanly.  --help and --version use this path.
// It is converted to exit code 0 at the [run] / [newApp] boundary.
const exitHandled = -1

// run is the top-level orchestrator.  It creates an [app], calls each phase
// method in order, and translates errors into a numeric exit code.  The
// entire flow is visible in one screen.
func run() int {
	a, code := newApp(os.Args[1:])
	if code == exitHandled {
		return 0 // --help / --version: handled, exit cleanly
	}
	if code != 0 {
		return code
	}
	defer a.cleanup()

	if err := a.acquireLock(); err != nil {
		a.fail(err)
		return a.exitCode
	}

	if err := a.loadConfig(); err != nil {
		a.fail(err)
		return a.exitCode
	}
	if err := a.loadSecrets(); err != nil {
		a.fail(err)
		return a.exitCode
	}

	a.printHeader()
	a.printSummary()

	if err := a.execute(); err != nil {
		a.fail(err)
		return a.exitCode
	}

	a.log.Info("All operations completed.")
	return 0
}

// ---------------------------------------------------------------------------
// newApp – construction and early validation
// ---------------------------------------------------------------------------

// newApp parses CLI arguments, initialises the logger, validates the
// environment (root, required binaries, label safety), derives all mode
// booleans and filesystem paths, and installs signal handling.
//
// The initialisation is decomposed into focused sub-initializers that are
// called in a fixed sequence:
//
//	initLogger → parseFlags → validateEnvironment → derivePaths → installSignalHandler
//
// Each sub-initializer has a single responsibility and is independently
// testable.  See the godoc on each method for details.
//
// It returns the initialised *app and exit code 0 on success.
// On failure it returns nil and a non-zero exit code (the caller should
// return that code directly without calling cleanup).
func newApp(args []string) (*app, int) {
	a := &app{}

	if code := a.initLogger(); code != 0 {
		return nil, code
	}
	if code := a.parseAppFlags(args); code != 0 {
		return nil, code // exitHandled for --help/--version, 1 for parse errors
	}
	if err := a.validateEnvironment(); err != nil {
		a.log.Error("%v", err)
		return nil, 1
	}
	if err := a.derivePaths(); err != nil {
		a.log.Error("%v", err)
		return nil, 1
	}

	a.installSignalHandler()
	return a, 0
}

// ---------------------------------------------------------------------------
// Sub-initializers – called by newApp in sequence
// ---------------------------------------------------------------------------

// initLogger creates the structured logger for the application.
//
// It auto-detects colour support based on whether stderr is a terminal,
// then creates a [logger.Logger] that writes both to stderr and to a
// timestamped log file under [logDir].
//
// On success it sets a.log and returns exit code 0.
// On failure it writes a raw error to stderr and returns exit code 1.
// This is the only initializer that must fall back to raw stderr on
// error because the logger itself is not yet available.
func (a *app) initLogger() int {
	enableColour := logger.IsTerminal(os.Stderr)
	log, err := logger.New(logDir, scriptName, enableColour)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to initialise logger: %v\n", err)
		return 1
	}
	a.log = log
	return 0
}

// parseAppFlags checks for --help/--version early-exit flags and then
// parses all remaining CLI arguments into a [flags] struct.
//
// The --help and --version checks are performed before any
// privilege/dependency checks so that help/version text is always
// accessible regardless of the runtime environment.
//
// On success it sets a.flags and the derived mode booleans
// (doBackup, doPrune, deepPruneMode, fixPermsOnly) and returns 0.
// For --help or --version it prints output and returns 0 (early exit).
// On parse errors it logs the error, prints usage, and returns 1.
//
// Note: this method is named parseAppFlags (rather than parseFlags) to
// avoid shadowing the package-level [parseFlags] function which handles
// the raw argument parsing.
func (a *app) parseAppFlags(args []string) int {
	// Check for --help and --version before any privilege/dependency checks
	// so that help/version text is always accessible regardless of environment.
	// These return exitHandled (not 0) so that newApp() and run() can
	// distinguish "handled, stop cleanly" from "success, continue".
	for _, arg := range args {
		if arg == "--help" {
			printUsage()
			return exitHandled
		}
		if arg == "--version" || arg == "-v" {
			fmt.Printf("%s %s (built %s)\n", scriptName, version, buildTime)
			return exitHandled
		}
	}

	f, err := parseFlags(args)
	if err != nil {
		a.log.Error("%v", err)
		fmt.Fprintln(os.Stderr)
		printUsage()
		a.log.Close()
		return 1
	}

	a.flags = f

	// Derive mode booleans from the parsed flags.
	a.doBackup = f.mode == "backup"
	a.doPrune = f.mode == "prune" || f.mode == "prune-deep"
	a.deepPruneMode = f.mode == "prune-deep"
	a.fixPermsOnly = f.fixPerms && !a.doBackup && !a.doPrune
	a.backupLabel = f.source

	return 0
}

// validateEnvironment checks runtime prerequisites that must be satisfied
// before any filesystem operations or command execution.
//
// It validates, in order:
//  1. Root privilege – the process must be running as UID 0.
//  2. Label safety – the source label must match [labelPattern] to prevent
//     path traversal attacks (labels are interpolated into filesystem paths).
//  3. Binary dependencies – 'duplicacy' is required for backup/prune modes;
//     'btrfs' is required for backup mode (snapshot creation).
//  4. Flag combination rules – e.g. --prune-deep requires --force-prune,
//     --force-prune requires a prune mode, --fix-perms is local-only.
//  5. Default mode logging – informs the user when no mode was explicitly
//     specified.
//
// Returns nil on success.  On failure returns a descriptive error; the
// caller logs it and exits.
func (a *app) validateEnvironment() error {
	// Must be root
	if os.Geteuid() != 0 {
		a.log.Close()
		return fmt.Errorf("Must be run as root.")
	}

	// Validate label before any filesystem operations (security: prevent path traversal)
	if err := validateLabel(a.flags.source); err != nil {
		a.log.Close()
		return fmt.Errorf("Invalid source label: %v", err)
	}

	// Check duplicacy binary – only needed for backup or prune operations.
	// Skip for standalone --fix-perms which only calls chown/chmod.
	if a.doBackup || a.doPrune {
		if _, err := exec.LookPath("duplicacy"); err != nil {
			a.log.Close()
			return fmt.Errorf("Required command 'duplicacy' not found")
		}
	}

	// Check btrfs command – only needed for backup (snapshot create/delete)
	if a.doBackup {
		if _, err := exec.LookPath("btrfs"); err != nil {
			a.log.Close()
			return fmt.Errorf("Required command 'btrfs' not found (needed for backup snapshots)")
		}
	}

	// Validate flag combinations
	if a.deepPruneMode && !a.flags.forcePrune {
		a.log.Close()
		return fmt.Errorf("--prune-deep requires --force-prune")
	}

	if a.flags.forcePrune && !a.doPrune {
		a.log.Close()
		return fmt.Errorf("--force-prune requires --prune or --prune-deep")
	}

	if a.flags.fixPerms && a.flags.remoteMode {
		a.log.Close()
		return fmt.Errorf("--fix-perms is only valid for local backups; cannot be used with --remote")
	}

	if a.flags.mode == "" {
		if a.flags.fixPerms {
			a.log.Info("No primary mode specified: using fix-perms only.")
		} else {
			a.log.Info("No mode specified: defaulting to backup only.")
		}
	}

	return nil
}

// derivePaths computes all filesystem paths, timestamps, and directory
// locations needed by the application.  It also creates the shared
// [execpkg.CommandRunner] and the directory-based [lock.Lock].
//
// Paths derived:
//   - snapshotSource: /volume1/<label>
//   - snapshotTarget: /volume1/<label>-<timestamp>
//   - workRoot: <tmpdir>/duplicacy-backup-<label>-<timestamp>-<pid>
//   - repositoryPath: snapshotTarget (backup) or snapshotSource (prune/fix-perms)
//   - configDir: resolved from flag → env var → executable-relative default
//   - configFile: <configDir>/<label>-backup.conf
//   - secretsDir: resolved from flag → env var → default
//
// Returns nil on success.  Currently this method cannot fail, but the
// error return allows future validation (e.g. path length checks) without
// changing the caller.
func (a *app) derivePaths() error {
	a.runTimestamp = time.Now().Format("20060102-150405")
	a.snapshotSource = filepath.Join(rootVolume, a.backupLabel)
	a.snapshotTarget = filepath.Join(rootVolume, fmt.Sprintf("%s-%s", a.backupLabel, a.runTimestamp))
	a.workRoot = filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s-%s-%d", scriptName, a.backupLabel, a.runTimestamp, os.Getpid()))

	if a.doBackup {
		a.repositoryPath = a.snapshotTarget
	} else {
		a.repositoryPath = a.snapshotSource
	}

	// Resolve config and secrets directories
	a.configDir = resolveDir(a.flags.configDir, "DUPLICACY_BACKUP_CONFIG_DIR", executableConfigDir())
	a.configFile = filepath.Join(a.configDir, fmt.Sprintf("%s-backup.conf", a.backupLabel))
	a.secretsDir = resolveDir(a.flags.secretsDir, "DUPLICACY_BACKUP_SECRETS_DIR", config.DefaultSecretsDir)

	// Create the shared command runner used by btrfs, duplicacy, and permissions.
	a.runner = execpkg.NewCommandRunner(a.log, a.flags.dryRun)

	// Create the directory-based PID lock.
	a.lk = lock.New(lockParent, a.backupLabel)

	return nil
}

// installSignalHandler sets up a goroutine that listens for SIGINT,
// SIGHUP, and SIGTERM.  On receipt of any of these signals the handler
// logs a warning, sets the exit code to 1, calls [app.cleanup], and
// terminates the process.
//
// This ensures that btrfs snapshots and temporary work directories are
// cleaned up even when the process is interrupted.
func (a *app) installSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		a.log.Warn("Received signal: %v — initiating cleanup.", sig)
		a.exitCode = 1
		a.cleanup()
		os.Exit(1)
	}()
}

// ---------------------------------------------------------------------------
// Phase methods
// ---------------------------------------------------------------------------

// acquireLock creates and acquires the directory-based PID lock for the
// current backup label.
func (a *app) acquireLock() error {
	a.log.Info("Acquiring lock for label %q.", a.backupLabel)
	if err := a.lk.Acquire(); err != nil {
		return fmt.Errorf("Lock acquisition failed: %w", err)
	}
	a.lockAcquired = true
	a.log.Info("Lock acquired: %s.", a.lk.Path)
	return nil
}

// loadConfig parses the INI config file, applies values, validates required
// fields and thresholds, builds prune arguments, computes backupTarget,
// and performs mode-specific validation (threads, owner/group, btrfs volumes).
func (a *app) loadConfig() error {
	a.log.Info("Loading configuration from %s.", a.configFile)

	if _, err := os.Stat(a.configFile); os.IsNotExist(err) {
		return fmt.Errorf("Configuration file not found: %s.", a.configFile)
	}

	// Parse config
	cfg := config.NewDefaults()

	targetSection := "local"
	if a.flags.remoteMode {
		targetSection = "remote"
	}

	values, err := config.ParseFile(a.configFile, targetSection)
	if err != nil {
		return err
	}
	if err := cfg.Apply(values); err != nil {
		return err
	}

	a.log.Info("Configuration parsed for [common] + [%s].", targetSection)

	// Clean up old logs
	a.log.CleanupOldLogs(cfg.LogRetentionDays, a.flags.dryRun)

	// Validate config
	if err := cfg.ValidateRequired(a.doBackup, a.doPrune); err != nil {
		return err
	}

	if err := cfg.ValidateThresholds(); err != nil {
		return err
	}

	// LOCAL_OWNER and LOCAL_GROUP are only needed when --fix-perms will
	// actually change file ownership.  Skip the (potentially expensive)
	// user/group look-ups for plain backup or prune runs.
	if a.flags.fixPerms {
		if err := cfg.ValidateOwnerGroup(); err != nil {
			return err
		}
	}

	cfg.BuildPruneArgs()

	if a.doBackup {
		if err := cfg.ValidateThreads(); err != nil {
			return err
		}
	}

	// Check btrfs volumes – only needed for backup (snapshot creation)
	if a.doBackup {
		if err := btrfs.CheckVolume(a.runner, rootVolume, a.flags.dryRun); err != nil {
			return err
		}
		a.log.Info("Verified '%s' is on a btrfs filesystem.", rootVolume)

		if err := btrfs.CheckVolume(a.runner, a.snapshotSource, a.flags.dryRun); err != nil {
			return err
		}
		a.log.Info("Verified '%s' is on a btrfs filesystem.", a.snapshotSource)
	}

	a.cfg = cfg
	a.backupTarget = joinDestination(cfg.Destination, a.backupLabel)
	a.log.Info("Configuration loaded successfully.")
	return nil
}

// loadSecrets loads and validates the secrets file for remote mode.
// For local mode this is a no-op.
func (a *app) loadSecrets() error {
	if !a.flags.remoteMode {
		return nil
	}

	secretsFile := secrets.GetSecretsFilePath(a.secretsDir, config.DefaultSecretsPrefix, a.backupLabel)
	a.log.Info("Loading secrets from %s.", secretsFile)

	sec, err := secrets.LoadSecretsFile(secretsFile)
	if err != nil {
		return err
	}
	if err := sec.Validate(); err != nil {
		return err
	}
	a.sec = sec
	a.log.Info("Secrets loaded and validated.")
	return nil
}

// printHeader emits the startup banner with script name, PID, and lock path.
func (a *app) printHeader() {
	a.log.PrintSeparator()
	a.log.Info("Backup script started - %s", time.Now().Format("2006-01-02 15:04:05"))
	a.log.PrintLine("Script", scriptName)
	a.log.PrintLine("PID", fmt.Sprintf("%d", os.Getpid()))
	a.log.PrintLine("Lock Path", a.lk.Path)
	a.log.PrintSeparator()
}

// printSummary emits the configuration summary.  The output varies depending
// on the operation mode: standalone fix-perms gets a minimal summary while
// backup/prune modes get the full field set.
func (a *app) printSummary() {
	modeStr := "LOCAL"
	if a.flags.remoteMode {
		modeStr = "REMOTE"
	}

	// Determine operation mode string
	var opMode string
	if a.fixPermsOnly {
		opMode = "Fix permissions only"
	} else if a.doBackup && a.flags.fixPerms {
		opMode = "Backup + fix permissions"
	} else if a.doBackup {
		opMode = "Backup only"
	} else if a.doPrune && a.deepPruneMode && a.flags.fixPerms {
		opMode = "Prune deep + fix permissions"
	} else if a.doPrune && a.deepPruneMode {
		opMode = "Prune deep"
	} else if a.doPrune && a.flags.fixPerms {
		opMode = "Prune safe + fix permissions"
	} else if a.doPrune {
		opMode = "Prune safe"
	}

	a.log.Info("Configuration Summary:")
	a.log.PrintLine("Operation Mode", opMode)

	if a.fixPermsOnly {
		// Match the bash script's standalone fix-perms summary layout.
		a.log.PrintLine("Destination", a.backupTarget)
		a.log.PrintLine("Local Owner", a.cfg.LocalOwner)
		a.log.PrintLine("Local Group", a.cfg.LocalGroup)
		a.log.PrintLine("Dry Run", fmt.Sprintf("%t", a.flags.dryRun))
	} else {
		// Match the bash script's full summary field ordering and labels.
		a.log.PrintLine("Config File", a.configFile)
		a.log.PrintLine("Backup Label", a.backupLabel)
		a.log.PrintLine("Mode", modeStr)
		a.log.PrintLine("Source", a.snapshotSource)
		if a.doBackup {
			a.log.PrintLine("Snapshot", a.repositoryPath)
		}
		a.log.PrintLine("Work Dir", filepath.Join(a.workRoot, "duplicacy"))
		a.log.PrintLine("Destination", a.backupTarget)
		if a.cfg.Threads > 0 {
			a.log.PrintLine("Threads", fmt.Sprintf("%d", a.cfg.Threads))
		} else {
			a.log.PrintLine("Threads", "<n/a>")
		}
		if a.cfg.Filter != "" {
			a.log.PrintLine("Filter", a.cfg.Filter)
		} else {
			a.log.PrintLine("Filter", "<none>")
		}
		if a.cfg.Prune != "" {
			a.log.PrintLine("Prune Options", a.cfg.Prune)
		} else {
			a.log.PrintLine("Prune Options", "<none>")
		}

		a.log.PrintLine("Log Retention", fmt.Sprintf("%d", a.cfg.LogRetentionDays))
		a.log.PrintLine("Dry Run", fmt.Sprintf("%t", a.flags.dryRun))
		a.log.PrintLine("Force Prune", fmt.Sprintf("%t", a.flags.forcePrune))
		a.log.PrintLine("Fix Perms", fmt.Sprintf("%t", a.flags.fixPerms))
		a.log.PrintLine("Prune Max %", fmt.Sprintf("%d", a.cfg.SafePruneMaxDeletePercent))
		a.log.PrintLine("Prune Max Count", fmt.Sprintf("%d", a.cfg.SafePruneMaxDeleteCount))
		a.log.PrintLine("Prune Min Total Revs", fmt.Sprintf("%d", a.cfg.SafePruneMinTotalForPercent))

		// Only show Local Owner/Group when --fix-perms is active
		// (these fields are not relevant for plain backup or prune).
		if a.flags.fixPerms {
			a.log.PrintLine("Local Owner", a.cfg.LocalOwner)
			a.log.PrintLine("Local Group", a.cfg.LocalGroup)
		}

		if a.flags.remoteMode && a.sec != nil {
			a.log.PrintLine("Secrets Dir", a.secretsDir)
			a.log.PrintLine("Secrets File", secrets.GetSecretsFilePath(a.secretsDir, config.DefaultSecretsPrefix, a.backupLabel))
			a.log.PrintLine("STORJ S3 ID", a.sec.MaskedID())
			a.log.PrintLine("STORJ S3 Secret", a.sec.MaskedSecret())
		}
	}
}

// execute dispatches to the appropriate phase methods based on the operation
// mode flags.  It runs backup/prune phases first, then fix-perms if requested.
func (a *app) execute() error {
	if a.doBackup || a.doPrune {
		if err := a.prepareDuplicacySetup(); err != nil {
			return err
		}
	}

	if a.doBackup {
		if err := a.runBackupPhase(); err != nil {
			return err
		}
	}

	if a.doPrune {
		if err := a.runPrunePhase(); err != nil {
			return err
		}
	}

	if a.flags.fixPerms {
		if err := a.runFixPermsPhase(); err != nil {
			return err
		}
	}

	return nil
}

// prepareDuplicacySetup creates the btrfs snapshot (for backup mode),
// initialises the duplicacy working environment, writes preferences and
// filter files, and sets directory permissions.
func (a *app) prepareDuplicacySetup() error {
	// Create btrfs snapshot for backup
	if a.doBackup {
		a.log.Info("Creating btrfs snapshot: %s → %s.", a.snapshotSource, a.snapshotTarget)
		if a.flags.dryRun {
			a.log.DryRun("btrfs subvolume snapshot -r %s %s", a.snapshotSource, a.snapshotTarget)
		} else {
			if err := btrfs.CreateSnapshot(a.runner, a.snapshotSource, a.snapshotTarget, a.flags.dryRun); err != nil {
				return fmt.Errorf("Failed to create snapshot: %w.", err)
			}
			a.log.Info("Snapshot created successfully.")
		}
	}

	// Set up duplicacy working environment
	dup := duplicacy.NewSetup(a.workRoot, a.repositoryPath, a.backupTarget, a.flags.dryRun, a.runner)

	if err := dup.CreateDirs(); err != nil {
		return err
	}
	if a.flags.dryRun {
		a.log.DryRun("mkdir -p %s", dup.DuplicacyDir)
	}

	// Write preferences
	if err := dup.WritePreferences(a.sec); err != nil {
		return fmt.Errorf("Failed to write preferences: %w", err)
	}
	if a.flags.dryRun {
		a.log.DryRun("write JSON preferences to %s", dup.PrefsFile)
	}

	// Write filters for backup mode
	if a.doBackup && a.cfg.Filter != "" {
		a.log.Info("Creating filter definitions.")
		if err := dup.WriteFilters(a.cfg.Filter); err != nil {
			return err
		}
		if a.flags.dryRun {
			a.log.DryRun("Write filters to %s", dup.FilterFile)
			for _, line := range strings.Split(a.cfg.Filter, "\n") {
				a.log.Info("  %s", line)
			}
		} else {
			a.log.Info("Active filters:")
			for _, line := range strings.Split(a.cfg.Filter, "\n") {
				a.log.Info("  %s", line)
			}
		}
	}

	// Set permissions on work directory
	if err := dup.SetPermissions(); err != nil {
		return err
	}
	if a.flags.dryRun {
		a.log.DryRun("find %s -type d -exec chmod 770 {} +", dup.DuplicacyRoot)
		a.log.DryRun("find %s -type f -exec chmod 660 {} +", dup.DuplicacyRoot)
	}

	a.log.Info("Changing to directory: %s.", dup.DuplicacyRoot)
	a.dup = dup
	return nil
}

// runBackupPhase executes the duplicacy backup command.  The coordinator
// wraps the operation with high-level phase messages and pipes Duplicacy's
// native output through the logger.
func (a *app) runBackupPhase() error {
	a.log.Info("Starting backup phase.")

	if a.flags.dryRun {
		a.log.DryRun("duplicacy backup -stats -threads %d", a.cfg.Threads)
		a.log.Info("Backup phase completed (dry-run).")
		return nil
	}

	stdout, stderr, err := a.dup.RunBackup(a.cfg.Threads)
	a.printCommandOutput(stdout, stderr)

	if err != nil {
		return fmt.Errorf("Backup failed: %w.", err)
	}

	a.log.Info("Backup phase completed successfully.")
	return nil
}

// runPrunePhase validates the repository, runs a safe prune preview,
// enforces threshold guards, executes the policy prune, and optionally
// runs a deep (exhaustive + exclusive) prune.
func (a *app) runPrunePhase() error {
	a.log.Info("Starting prune phase.")

	// Validate repository
	if a.flags.dryRun {
		a.log.DryRun("duplicacy list -files")
	} else {
		if err := a.dup.ValidateRepo(); err != nil {
			return fmt.Errorf("Cannot perform prune operation — repository not ready: %w.", err)
		}
		a.log.Info("Duplicacy repository validated.")
	}

	// Run safe prune preview
	if a.flags.dryRun {
		a.log.DryRun("duplicacy prune %s -dry-run", strings.Join(a.cfg.PruneArgs, " "))
	}
	preview, err := a.dup.SafePrunePreview(a.cfg.PruneArgs, a.cfg.SafePruneMinTotalForPercent)
	if err != nil {
		return fmt.Errorf("Safe prune preview failed: %w.", err)
	}

	// Log preview output
	if preview.Output != "" {
		for _, line := range strings.Split(preview.Output, "\n") {
			if line != "" {
				a.log.Info("[SAFE-PRUNE-PREVIEW] %s", line)
			}
		}
	}
	if preview.RevisionOutput != "" {
		for _, line := range strings.Split(preview.RevisionOutput, "\n") {
			if line != "" {
				a.log.Info("[REVISION-LIST] %s", line)
			}
		}
	}

	// Fail-closed: if revision count failed, block unless --force-prune
	if preview.RevisionCountFailed {
		if a.flags.forcePrune {
			a.log.Warn("Revision count failed; proceeding because --force-prune was supplied (percentage threshold not enforced).")
		} else {
			return fmt.Errorf("Revision count is required for safe prune but failed; use --force-prune to override.")
		}
	}

	// Display preview
	a.log.PrintLine("Preview Deletes", fmt.Sprintf("%d", preview.DeleteCount))
	a.log.PrintLine("Preview Total Revs", fmt.Sprintf("%d", preview.TotalRevisions))
	if preview.PercentEnforced {
		a.log.PrintLine("Preview Delete %", fmt.Sprintf("%d", preview.DeletePercent))
	} else {
		a.log.PrintLine("Preview Delete %", fmt.Sprintf("<not enforced; total revisions unavailable or below %d>", a.cfg.SafePruneMinTotalForPercent))
	}

	// Check thresholds
	blocked := false
	if preview.DeleteCount > a.cfg.SafePruneMaxDeleteCount {
		a.log.Error("Safe prune preview exceeds delete count threshold: %d > %d.", preview.DeleteCount, a.cfg.SafePruneMaxDeleteCount)
		blocked = true
	}
	if preview.ExceedsPercent(a.cfg.SafePruneMaxDeletePercent) {
		a.log.Error("Safe prune preview exceeds delete percentage threshold (%d of %d revisions > %d%%).", preview.DeleteCount, preview.TotalRevisions, a.cfg.SafePruneMaxDeletePercent)
		blocked = true
	}

	if blocked {
		if a.flags.forcePrune {
			a.log.Warn("Proceeding despite safe prune threshold breach because --force-prune was supplied.")
		} else {
			return fmt.Errorf("Refusing to continue because safe prune thresholds were exceeded.")
		}
	}

	// Execute policy prune
	a.log.Info("Starting policy prune.")
	if a.flags.dryRun {
		a.log.DryRun("duplicacy prune %s", strings.Join(a.cfg.PruneArgs, " "))
	} else {
		stdout, stderr, err := a.dup.RunPrune(a.cfg.PruneArgs)
		a.printCommandOutput(stdout, stderr)
		if err != nil {
			return fmt.Errorf("Policy prune failed: %w.", err)
		}
	}
	a.log.Info("Policy prune completed.")

	// Deep prune
	if a.deepPruneMode {
		a.log.Warn("Starting deep prune maintenance step: duplicacy prune -exhaustive -exclusive.")
		if a.flags.dryRun {
			a.log.DryRun("duplicacy prune -exhaustive -exclusive")
		} else {
			stdout, stderr, err := a.dup.RunDeepPrune()
			a.printCommandOutput(stdout, stderr)
			if err != nil {
				return fmt.Errorf("Deep prune failed: %w.", err)
			}
		}
		a.log.Info("Deep prune completed.")
	}

	a.log.Info("Prune phase completed successfully.")
	return nil
}

// runFixPermsPhase normalises ownership and permissions on the local backup
// target directory.
func (a *app) runFixPermsPhase() error {
	a.log.Info("Starting permission normalisation on %s.", a.backupTarget)
	a.log.PrintLine("Fix Perms Path", a.backupTarget)
	a.log.PrintLine("Fix Perms Owner", a.cfg.LocalOwner)
	a.log.PrintLine("Fix Perms Group", a.cfg.LocalGroup)

	if a.flags.dryRun {
		ownerGroup := fmt.Sprintf("%s:%s", a.cfg.LocalOwner, a.cfg.LocalGroup)
		a.log.DryRun("chown -R %s %s", ownerGroup, a.backupTarget)
		a.log.DryRun("find %s -type d -exec chmod 770 {} +", a.backupTarget)
		a.log.DryRun("find %s -type f -exec chmod 660 {} +", a.backupTarget)
		a.log.Info("Permission normalisation completed (dry-run).")
		return nil
	}

	if err := permissions.Fix(a.runner, a.backupTarget, a.cfg.LocalOwner, a.cfg.LocalGroup, a.flags.dryRun); err != nil {
		return fmt.Errorf("Permission normalisation failed: %w.", err)
	}

	a.log.Info("Permission normalisation completed successfully.")
	return nil
}

// ---------------------------------------------------------------------------
// cleanup and fail
// ---------------------------------------------------------------------------

// cleanup performs idempotent end-of-run cleanup: deletes the btrfs snapshot,
// removes the duplicacy work directory, releases the lock (only if it was
// successfully acquired), and prints the final result banner.  It is safe to
// call multiple times (e.g. from both defer and signal handler) and safe to
// call even when lock acquisition failed or was never attempted.
func (a *app) cleanup() {
	if a.cleanedUp {
		return
	}
	a.cleanedUp = true

	a.log.Info("Starting cleanup.")

	if a.doBackup {
		if _, err := os.Stat(a.snapshotTarget); err == nil {
			a.log.Info("Deleting snapshot subvolume: %s.", a.snapshotTarget)
			if a.flags.dryRun {
				a.log.DryRun("btrfs subvolume delete %s", a.snapshotTarget)
			} else {
				if delErr := btrfs.DeleteSnapshot(a.runner, a.snapshotTarget, a.flags.dryRun); delErr != nil {
					a.log.Warn("Failed to delete subvolume %s: %v.", a.snapshotTarget, delErr)
				}
			}
		}

		if _, err := os.Stat(a.snapshotTarget); err == nil {
			a.log.Info("Removing snapshot directory: %s.", a.snapshotTarget)
			if a.flags.dryRun {
				a.log.DryRun("rm -rf %s", a.snapshotTarget)
			} else {
				if rmErr := os.RemoveAll(a.snapshotTarget); rmErr != nil {
					a.log.Warn("Failed to remove snapshot directory %s.", a.snapshotTarget)
				}
			}
		}
	}

	if a.dup != nil {
		a.log.Info("Removing duplicacy work directory: %s.", a.dup.WorkRoot)
		if a.flags.dryRun {
			a.log.DryRun("rm -rf %s", a.dup.WorkRoot)
		} else {
			if err := a.dup.Cleanup(); err != nil {
				a.log.Warn("Failed to remove work directory: %v.", err)
			}
		}
	} else if _, err := os.Stat(a.workRoot); err == nil {
		a.log.Info("Removing duplicacy work directory: %s.", a.workRoot)
		if a.flags.dryRun {
			a.log.DryRun("rm -rf %s", a.workRoot)
		} else {
			if rmErr := os.RemoveAll(a.workRoot); rmErr != nil {
				a.log.Warn("Failed to remove work directory %s: %v.", a.workRoot, rmErr)
			}
		}
	}

	if a.lockAcquired {
		a.lk.Release()
	}

	status := "SUCCESS"
	if a.exitCode != 0 {
		status = "FAILED"
	}

	a.log.PrintSeparator()
	a.log.Info("Backup script completed:")
	a.log.PrintLine("Result", a.log.FormatResult(status))
	a.log.PrintLine("Code", fmt.Sprintf("%d", a.exitCode))
	a.log.PrintLine("Timestamp", time.Now().Format("2006-01-02 15:04:05"))
	a.log.PrintSeparator()

	a.log.Close()
}

// fail logs an error and sets the exit code to 1.
func (a *app) fail(err error) {
	a.log.Error("%v", err)
	a.exitCode = 1
}

// printCommandOutput pipes stdout and stderr from an external command
// through the logger, line by line.  This ensures all output appears in
// both the console and log file.
func (a *app) printCommandOutput(stdout, stderr string) {
	if stdout != "" {
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line != "" {
				a.log.Info("%s", line)
			}
		}
	}
	if stderr != "" {
		for _, line := range strings.Split(strings.TrimRight(stderr, "\n"), "\n") {
			if line != "" {
				a.log.Warn("%s", line)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Free functions – pure or side-effect-free, no app state needed
// ---------------------------------------------------------------------------

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

// parseFlags parses command-line arguments into a [flags] struct.
// It handles mode flags, modifier flags, and the positional source argument.
// --help and --version are handled before parseFlags is called (in [newApp]).
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

// printUsage writes the full help text to stdout.
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
    Default: %s/<source>-backup.conf
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
