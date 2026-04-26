package workflow

import (
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

const (
	locationLocal  = "local"
	locationRemote = "remote"
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
	UserLookup    func(string) (*user.User, error)
}

type UserProfileDirs struct {
	ConfigDir  string
	SecretsDir string
	LogDir     string
	StateDir   string
	LockDir    string
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
		UserLookup: user.Lookup,
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

// DefaultMetadata returns metadata rooted around an explicit log directory.
//
// Production entry points should use DefaultMetadataForRuntime so defaults
// follow the invoking user's runtime profile. This helper remains useful for
// tests and other callers that need deterministic sibling state/lock roots.
func DefaultMetadata(scriptName, version, buildTime, logDir string) Metadata {
	baseDir := filepath.Dir(logDir)
	logName := filepath.Base(logDir)
	stateRoot := filepath.Join(baseDir, logName+"-state")
	lockRoot := filepath.Join(baseDir, logName+"-locks")
	return Metadata{
		ScriptName: scriptName,
		Version:    version,
		BuildTime:  buildTime,
		RootVolume: "/volume1",
		LockParent: lockRoot,
		LogDir:     logDir,
		StateDir:   stateRoot,
	}
}

func DefaultMetadataForRuntime(scriptName, version, buildTime string, rt Runtime) Metadata {
	dirs := DefaultUserProfileDirs(rt)
	return Metadata{
		ScriptName: scriptName,
		Version:    version,
		BuildTime:  buildTime,
		RootVolume: "/volume1",
		LockParent: dirs.LockDir,
		LogDir:     dirs.LogDir,
		StateDir:   dirs.StateDir,
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

func ResolveDir(rt Runtime, flagValue, envVar, defaultDir string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := runtimeEnv(rt, envVar); v != "" {
		return v
	}
	return defaultDir
}

func DefaultUserProfileDirs(rt Runtime) UserProfileDirs {
	configRoot := runtimeEnv(rt, "XDG_CONFIG_HOME")
	if configRoot == "" {
		configRoot = filepath.Join(userHomeDir(rt), ".config")
	}
	stateRoot := runtimeEnv(rt, "XDG_STATE_HOME")
	if stateRoot == "" {
		stateRoot = filepath.Join(userHomeDir(rt), ".local", "state")
	}
	appConfig := filepath.Join(configRoot, "duplicacy-backup")
	appState := filepath.Join(stateRoot, "duplicacy-backup")
	return UserProfileDirs{
		ConfigDir:  appConfig,
		SecretsDir: filepath.Join(appConfig, "secrets"),
		LogDir:     filepath.Join(appState, "logs"),
		StateDir:   filepath.Join(appState, "state"),
		LockDir:    filepath.Join(appState, "locks"),
	}
}

func userHomeDir(rt Runtime) string {
	if home := sudoOperatorHomeDir(rt); home != "" {
		return home
	}
	if home := runtimeEnv(rt, "HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

func sudoOperatorHomeDir(rt Runtime) string {
	if runtimeEUID(rt) != 0 {
		return ""
	}
	sudoUser, ok := sudoOperatorUser(rt)
	if !ok {
		return ""
	}
	lookup := rt.UserLookup
	if lookup == nil {
		lookup = user.Lookup
	}
	u, err := lookup(sudoUser)
	if err != nil || strings.TrimSpace(u.HomeDir) == "" {
		return ""
	}
	return u.HomeDir
}

func sudoOperatorUser(rt Runtime) (string, bool) {
	sudoUser := strings.TrimSpace(runtimeEnv(rt, "SUDO_USER"))
	if sudoUser == "" || sudoUser == "root" {
		return "", false
	}
	if _, err := strconv.ParseUint(strings.TrimSpace(runtimeEnv(rt, "SUDO_UID")), 10, 32); err != nil {
		return "", false
	}
	return sudoUser, true
}

func runtimeEUID(rt Runtime) int {
	if rt.Geteuid == nil {
		return os.Geteuid()
	}
	return rt.Geteuid()
}

func EffectiveConfigDir(rt Runtime) string {
	if v := runtimeEnv(rt, "DUPLICACY_BACKUP_CONFIG_DIR"); v != "" {
		return v
	}
	return DefaultUserProfileDirs(rt).ConfigDir
}

func EffectiveSecretsDir(rt Runtime) string {
	if v := runtimeEnv(rt, "DUPLICACY_BACKUP_SECRETS_DIR"); v != "" {
		return v
	}
	return DefaultUserProfileDirs(rt).SecretsDir
}

func runtimeEnv(rt Runtime, name string) string {
	if rt.Getenv == nil {
		return os.Getenv(name)
	}
	return rt.Getenv(name)
}

func SignalSet() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM}
}
