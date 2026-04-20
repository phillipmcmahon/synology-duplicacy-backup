package workflow

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

const (
	storageTypeDuplicacy = "duplicacy"
	locationLocal        = "local"
	locationRemote       = "remote"
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
	case storageTypeDuplicacy:
		return destination
	default:
		return filepath.Join(destination, label)
	}
}

func ResolveBackupTarget(storageType, destination, storage, repository string) string {
	if storageType == storageTypeDuplicacy {
		return storage
	}
	return JoinDestination(storageType, destination, repository)
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

func SignalSet() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
}
