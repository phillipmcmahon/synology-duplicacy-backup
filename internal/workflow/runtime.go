package workflow

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

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

const (
	storageTypeFilesystem = "filesystem"
	storageTypeObject     = "object"
	locationLocal         = "local"
	locationRemote        = "remote"
)

// Runtime provides the environment-facing functions used by request parsing,
// planning, and execution. Tests can replace individual functions without
// needing to stub whole packages.
type Runtime struct {
	Geteuid       func() int
	LookPath      func(string) (string, error)
	NewLock       func(string, string) *lock.Lock
	NewSourceLock func(string, string) *lock.Lock
	Now           func() time.Time
	TempDir       func() string
	Getpid        func() int
	Getenv        func(string) string
	Stdin         func() *os.File
	StdinIsTTY    func() bool
	Executable    func() (string, error)
	EvalSymlinks  func(string) (string, error)
	SignalNotify  func(chan<- os.Signal, ...os.Signal)
}

func DefaultRuntime() Runtime {
	return Runtime{
		Geteuid:       os.Geteuid,
		LookPath:      exec.LookPath,
		NewLock:       lock.New,
		NewSourceLock: lock.NewSource,
		Now:           time.Now,
		TempDir:       os.TempDir,
		Getpid:        os.Getpid,
		Getenv:        os.Getenv,
		Stdin:         func() *os.File { return os.Stdin },
		StdinIsTTY: func() bool {
			return logger.IsTerminal(os.Stdin)
		},
		Executable:   os.Executable,
		EvalSymlinks: filepath.EvalSymlinks,
		SignalNotify: func(ch chan<- os.Signal, sig ...os.Signal) {
			signal.Notify(ch, sig...)
		},
	}
}

// Metadata groups stable application metadata and default filesystem roots.
type Metadata struct {
	ScriptName string
	Version    string
	BuildTime  string
	RootVolume string
	LockParent string
	LogDir     string
	StateDir   string
}

func DefaultMetadata(scriptName, version, buildTime, logDir string) Metadata {
	return Metadata{
		ScriptName: scriptName,
		Version:    version,
		BuildTime:  buildTime,
		RootVolume: "/volume1",
		LockParent: "/var/lock",
		LogDir:     logDir,
		StateDir:   "/var/lib/duplicacy-backup",
	}
}

func ValidateLabel(label string) error {
	if label == "" {
		return NewRequestError("label must not be empty")
	}
	if strings.Contains(label, "/") || strings.Contains(label, "\\") || strings.Contains(label, "..") {
		return NewRequestError(
			"label %q contains path traversal characters (/, \\, or ..); only alphanumeric characters, hyphens, and underscores are allowed",
			label,
		)
	}
	if !labelPattern.MatchString(label) {
		return NewRequestError(
			"label %q contains invalid characters; only alphanumeric characters (a-z, A-Z, 0-9), hyphens (-), and underscores (_) are allowed, and must start with an alphanumeric character",
			label,
		)
	}
	return nil
}

func ValidateTargetName(target string) error {
	if target == "" {
		return NewRequestError("target must not be empty")
	}
	if strings.Contains(target, "/") || strings.Contains(target, "\\") || strings.Contains(target, "..") {
		return NewRequestError(
			"target %q contains path traversal characters (/, \\, or ..); only alphanumeric characters, hyphens, and underscores are allowed",
			target,
		)
	}
	if !targetPattern.MatchString(target) {
		return NewRequestError(
			"target %q contains invalid characters; only alphanumeric characters (a-z, A-Z, 0-9), hyphens (-), and underscores (_) are allowed, and must start with an alphanumeric character",
			target,
		)
	}
	return nil
}

func JoinDestination(storageType, destination, label string) string {
	switch storageType {
	case storageTypeObject:
		if idx := strings.Index(destination, "://"); idx >= 0 {
			scheme := destination[:idx+3]
			rest := strings.TrimRight(destination[idx+3:], "/")
			return scheme + rest + "/" + label
		}
		return destination
	default:
		return filepath.Join(destination, label)
	}
}

func ResolveDir(rt Runtime, flagValue, envVar, defaultDir string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := rt.Getenv(envVar); v != "" {
		return v
	}
	return defaultDir
}

func ExecutableConfigDir(rt Runtime) string {
	exe, err := rt.Executable()
	if err != nil {
		return filepath.Join(".", ".config")
	}
	exe, err = rt.EvalSymlinks(exe)
	if err != nil {
		return filepath.Join(".", ".config")
	}
	return filepath.Join(filepath.Dir(exe), ".config")
}

func EffectiveConfigDir(rt Runtime) string {
	if v := rt.Getenv("DUPLICACY_BACKUP_CONFIG_DIR"); v != "" {
		return v
	}
	return ExecutableConfigDir(rt)
}

func UsageText(meta Metadata, rt Runtime) string {
	return fmt.Sprintf(`Usage: %s [OPTIONS] <source>
       %s config <validate|explain|paths> [OPTIONS] <source>
       %s health <status|doctor|verify> [OPTIONS] <source>

Operations:
    --backup
    --prune
    --cleanup-storage
    --fix-perms

Execution order:
    backup -> prune -> cleanup-storage -> fix-perms

Common modifiers:
    --force-prune
    --target <name>
    --dry-run
    --verbose
    --json-summary
    --config-dir <path>
    --secrets-dir <path>
    --version, -v
    --help
    --help-full

Examples:
    %s --target onsite-usb --backup homes
    %s --target onsite-usb --backup --prune homes
    %s --target onsite-usb --json-summary --dry-run --backup homes
    %s --target offsite-storj --backup homes
    %s config validate --target onsite-usb homes
    %s health status --target onsite-usb homes

Use --help-full for the detailed reference.
`,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func FullUsageText(meta Metadata, rt Runtime) string {
	cfgDir := EffectiveConfigDir(rt)
	return fmt.Sprintf(`Usage: %s [OPTIONS] <source>
       %s config <validate|explain|paths> [OPTIONS] <source>
       %s health <status|doctor|verify> [OPTIONS] <source>

OPERATIONS:
    Operations may be combined. Execution order is fixed:
      1. backup
      2. prune
      3. cleanup-storage
      4. fix-perms

    --backup                 Request backup
    --prune                  Request threshold-guarded prune
    --cleanup-storage        Request storage maintenance:
                             duplicacy prune -exhaustive -exclusive
                             Use only when no other client is writing to the same storage
    --fix-perms              Normalise filesystem repository ownership and permissions
    At least one operation flag is required for runtime commands

MODIFIERS:
    --force-prune            Override safe prune thresholds during prune
    --target <name>          Perform operation against the named target config (required)
    --dry-run                Simulate actions without making changes
    --verbose                Show detailed operational logging and command details
    --json-summary           Write a machine-readable run summary to stdout
    --config-dir <path>      Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>     Override secrets directory (default: %s)
    --version, -v            Show version and build information
    --help                   Show the concise help message
    --help-full              Show the detailed help message

HEALTH COMMANDS:
    health status            Fast read-only health summary for operators and schedulers
    health doctor            Read-only environment and storage diagnostics
    health verify            Read-only integrity check across revisions found for the current label

ENVIRONMENT VARIABLES:
    DUPLICACY_BACKUP_CONFIG_DIR   Override config directory (--config-dir takes precedence)
    DUPLICACY_BACKUP_SECRETS_DIR  Override secrets directory (--secrets-dir takes precedence)

SAFE PRUNE THRESHOLDS:
    Max delete percent       : %d%% (default %d%%)
    Max delete count         : %d (default %d)
    Min revisions for %% check: %d (default %d)

CONFIG FILE LOCATION:
    <binary-dir>/.config/<label>-backup.toml
    Effective default: %s/<label>-backup.toml
    Override with --config-dir or DUPLICACY_BACKUP_CONFIG_DIR

CONFIG STRUCTURE:
    label config files define:
      source_path
      [common]
      [health]
      [health.notify]
      [targets.<name>]
      optional [targets.<name>.health]
      optional [targets.<name>.health.notify]
    each [targets.<name>] entry must include:
      type = "filesystem" | "object"
      location = "local" | "remote"

TARGET SECRETS:
    Object-storage targets load credentials from:
      %s/<label>-secrets.toml
    Filesystem targets do not use a secrets file, even when location = "remote"
    Override directory with --secrets-dir or DUPLICACY_BACKUP_SECRETS_DIR
    Use [targets.<name>] tables with:
      storj_s3_id
      storj_s3_secret
      optional health_webhook_bearer_token

HEALTH STATE:
    Target-specific run and health state are stored under:
      %s/<label>.<target>.json
    Health commands combine this state with live storage inspection.

HEALTH CONFIG:
    Optional [health] table keys:
      freshness_warn_hours
      freshness_fail_hours
      doctor_warn_after_hours
      verify_warn_after_hours

    Optional [health.notify] table keys:
      webhook_url
      notify_on = ["degraded", "unhealthy"]
      send_for = ["doctor", "verify"]  # add backup, prune, cleanup-storage to opt runtime alerts in
      interactive = false

    Optional secrets key:
      health_webhook_bearer_token

ARGUMENTS:
    source                   Backup label

INTERACTIVE SAFETY RAILS:
    Interactive terminal runs ask for confirmation before:
      - forced prune
      - cleanup-storage
    Non-interactive runs continue without confirmation so scheduled jobs are unaffected

JSON SUMMARY:
    --json-summary writes a machine-readable completion summary to stdout.
    Human-readable logs continue to be written to stderr.

EXAMPLES:
    %s --target onsite-usb --backup homes
    %s --target onsite-usb --backup homes
    %s --target onsite-usb --backup --prune homes
    %s --target onsite-usb --json-summary --dry-run --backup homes
    %s health status --target onsite-usb homes
    %s health doctor --json-summary --target onsite-usb homes
    %s health verify --target offsite-storj homes
    %s --target onsite-usb --prune homes
    %s --target onsite-usb --cleanup-storage homes
    %s --target onsite-usb --prune --cleanup-storage homes
    %s --target onsite-usb --prune --force-prune --cleanup-storage homes
    %s --target onsite-usb --backup --prune --force-prune --cleanup-storage homes
    %s --target onsite-usb --fix-perms homes
    %s --target onsite-usb --backup --fix-perms homes
    %s --target offsite-storj --backup homes
    %s --target onsite-usb --verbose --backup --prune homes
    %s --target onsite-usb --config-dir /opt/etc --backup homes
    %s --secrets-dir /opt/secrets --target offsite-storj --backup homes
    %s config validate --target onsite-usb homes
    %s config explain --target offsite-storj homes
    %s config paths --target onsite-usb homes
`,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		config.DefaultSecretsDir,
		config.DefaultSafePruneMaxDeletePercent, config.DefaultSafePruneMaxDeletePercent,
		config.DefaultSafePruneMaxDeleteCount, config.DefaultSafePruneMaxDeleteCount,
		config.DefaultSafePruneMinTotalForPercent, config.DefaultSafePruneMinTotalForPercent,
		cfgDir,
		config.DefaultSecretsDir,
		meta.StateDir,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName,
	)
}

func ConfigUsageText(meta Metadata, rt Runtime) string {
	return fmt.Sprintf(`Usage: %s config <validate|explain|paths> [OPTIONS] <source>

Config commands:
    validate
    explain
    paths

Options:
    --target <name>
    --config-dir <path>
    --secrets-dir <path>
    --help
    --help-full

Examples:
    %s config validate --target onsite-usb homes
    %s config explain --target offsite-storj homes
    %s config paths --target onsite-usb homes

Use --help-full for the detailed config reference.
`,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func FullConfigUsageText(meta Metadata, rt Runtime) string {
	cfgDir := EffectiveConfigDir(rt)
	return fmt.Sprintf(`Usage: %s config <validate|explain|paths> [OPTIONS] <source>

CONFIG COMMANDS:
    validate                Validate the resolved config and configured secrets
    explain                 Show the resolved config values for the selected target
    paths                   Show the resolved stable config, source, log, and any applicable secrets paths

OPTIONS:
    --target <name>         Select the named target (required)
    --config-dir <path>     Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>    Override secrets directory (default: %s)
    --help                  Show the concise help message
    --help-full             Show the detailed config help message

BEHAVIOUR:
    validate, explain, and paths operate on one selected target from a label config at a time.
    Every config command requires an explicit --target selection.

DEFAULT LOCATIONS:
    Config dir             : %s
    Secrets dir            : %s

EXAMPLES:
    %s config validate --target onsite-usb homes
    %s config validate --target offsite-storj homes
    %s config explain --target onsite-usb homes
    %s config explain --target offsite-storj homes
    %s config paths --target onsite-usb homes
`,
		meta.ScriptName,
		config.DefaultSecretsDir,
		cfgDir,
		config.DefaultSecretsDir,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func VersionText(meta Metadata) string {
	return fmt.Sprintf("%s %s (built %s)\n", meta.ScriptName, meta.Version, meta.BuildTime)
}

func SignalSet() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
}
