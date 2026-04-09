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
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Runtime provides the environment-facing functions used by request parsing,
// planning, and execution. Tests can replace individual functions without
// needing to stub whole packages.
type Runtime struct {
	Geteuid      func() int
	LookPath     func(string) (string, error)
	NewLock      func(string, string) *lock.Lock
	Now          func() time.Time
	TempDir      func() string
	Getpid       func() int
	Getenv       func(string) string
	Executable   func() (string, error)
	EvalSymlinks func(string) (string, error)
	SignalNotify func(chan<- os.Signal, ...os.Signal)
}

func DefaultRuntime() Runtime {
	return Runtime{
		Geteuid:      os.Geteuid,
		LookPath:     exec.LookPath,
		NewLock:      lock.New,
		Now:          time.Now,
		TempDir:      os.TempDir,
		Getpid:       os.Getpid,
		Getenv:       os.Getenv,
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
}

func DefaultMetadata(scriptName, version, buildTime, logDir string) Metadata {
	return Metadata{
		ScriptName: scriptName,
		Version:    version,
		BuildTime:  buildTime,
		RootVolume: "/volume1",
		LockParent: "/var/lock",
		LogDir:     logDir,
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

func JoinDestination(destination, label string) string {
	if idx := strings.Index(destination, "://"); idx >= 0 {
		scheme := destination[:idx+3]
		rest := strings.TrimRight(destination[idx+3:], "/")
		return scheme + rest + "/" + label
	}
	return filepath.Join(destination, label)
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
	cfgDir := EffectiveConfigDir(rt)
	return fmt.Sprintf(`Usage: %s [OPTIONS] <source>

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
    Effective default: %s/<source>-backup.conf
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
`,
		meta.ScriptName,
		config.DefaultSecretsDir,
		config.DefaultSafePruneMaxDeletePercent, config.DefaultSafePruneMaxDeletePercent,
		config.DefaultSafePruneMaxDeleteCount, config.DefaultSafePruneMaxDeleteCount,
		config.DefaultSafePruneMinTotalForPercent, config.DefaultSafePruneMinTotalForPercent,
		cfgDir,
		config.DefaultSecretsDir, config.DefaultSecretsPrefix,
		meta.RootVolume,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
	)
}

func VersionText(meta Metadata) string {
	return fmt.Sprintf("%s %s (built %s)\n", meta.ScriptName, meta.Version, meta.BuildTime)
}

func SignalSet() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
}
