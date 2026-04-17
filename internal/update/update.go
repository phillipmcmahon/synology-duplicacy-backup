package update

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

const (
	DefaultRepo                   = "phillipmcmahon/synology-duplicacy-backup"
	DefaultKeep                   = 2
	DefaultReleaseMetadataTimeout = 15 * time.Second
	DefaultAssetDownloadTimeout   = 10 * time.Minute
)

type Runtime struct {
	GOOS         string
	GOARCH       string
	Stdin        func() *os.File
	StdinIsTTY   func() bool
	CommandPath  func() string
	LookPath     func(string) (string, error)
	Executable   func() (string, error)
	EvalSymlinks func(string) (string, error)
	TempDir      func() string
	MkdirTemp    func(string, string) (string, error)
	RemoveAll    func(string) error
}

type plan struct {
	CurrentVersion string
	TargetVersion  string
	ReleaseTag     string
	AssetName      string
	AssetURL       string
	ChecksumURL    string
	InstallRoot    string
	BinDir         string
	CheckOnly      bool
	Force          bool
	Keep           int
	AlreadyCurrent bool
}

type Updater struct {
	Repo            string
	APIBase         string
	ScriptName      string
	CurrentVersion  string
	Runtime         Runtime
	HTTPClient      *http.Client
	RunInstaller    func(string, []string) ([]byte, error)
	ReleaseTimeout  time.Duration
	DownloadTimeout time.Duration
}

type Result struct {
	Output string
	Status Status
}

func DefaultRuntime() Runtime {
	return Runtime{
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Stdin:        func() *os.File { return os.Stdin },
		StdinIsTTY:   func() bool { return logger.IsTerminal(os.Stdin) },
		CommandPath:  func() string { return os.Args[0] },
		LookPath:     exec.LookPath,
		Executable:   os.Executable,
		EvalSymlinks: filepath.EvalSymlinks,
		TempDir:      os.TempDir,
		MkdirTemp:    os.MkdirTemp,
		RemoveAll:    os.RemoveAll,
	}
}

func New(scriptName, currentVersion string, rt Runtime) *Updater {
	if rt.Stdin == nil {
		rt.Stdin = func() *os.File { return os.Stdin }
	}
	if rt.StdinIsTTY == nil {
		rt.StdinIsTTY = func() bool { return logger.IsTerminal(os.Stdin) }
	}
	if rt.Executable == nil {
		rt.Executable = os.Executable
	}
	if rt.CommandPath == nil {
		rt.CommandPath = func() string { return os.Args[0] }
	}
	if rt.LookPath == nil {
		rt.LookPath = exec.LookPath
	}
	if rt.EvalSymlinks == nil {
		rt.EvalSymlinks = filepath.EvalSymlinks
	}
	if rt.TempDir == nil {
		rt.TempDir = os.TempDir
	}
	if rt.MkdirTemp == nil {
		rt.MkdirTemp = os.MkdirTemp
	}
	if rt.RemoveAll == nil {
		rt.RemoveAll = os.RemoveAll
	}
	if rt.GOOS == "" {
		rt.GOOS = runtime.GOOS
	}
	if rt.GOARCH == "" {
		rt.GOARCH = runtime.GOARCH
	}

	return &Updater{
		Repo:            DefaultRepo,
		APIBase:         "https://api.github.com",
		ScriptName:      scriptName,
		CurrentVersion:  strings.TrimPrefix(currentVersion, "v"),
		Runtime:         rt,
		HTTPClient:      http.DefaultClient,
		RunInstaller:    runInstallScript,
		ReleaseTimeout:  DefaultReleaseMetadataTimeout,
		DownloadTimeout: DefaultAssetDownloadTimeout,
	}
}

func (u *Updater) Run(options Options) (string, error) {
	result, err := u.RunResult(options)
	return result.Output, err
}

func (u *Updater) RunResult(options Options) (Result, error) {
	planned, err := u.buildPlan(options)
	if err != nil {
		return Result{Status: StatusFailed}, err
	}
	if planned.AlreadyCurrent {
		return Result{Output: renderReport(planned, "Already up to date", ""), Status: StatusCurrent}, nil
	}
	if planned.CheckOnly {
		result := "Update available"
		status := StatusAvailable
		if planned.Force && planned.TargetVersion == planned.CurrentVersion {
			result = "Reinstall requested"
			status = StatusReinstallRequested
		}
		return Result{Output: renderReport(planned, result, ""), Status: status}, nil
	}
	if err := u.confirmInstall(planned, options); err != nil {
		status := StatusFailed
		if strings.Contains(strings.ToLower(err.Error()), "cancelled") {
			status = StatusCancelled
		}
		return Result{Status: status}, err
	}
	installerOutput, err := u.install(planned)
	if err != nil {
		return Result{Status: StatusFailed}, err
	}
	return Result{Output: renderReport(planned, "Installed", installerOutput), Status: StatusInstalled}, nil
}

func (u *Updater) buildPlan(options Options) (*plan, error) {
	layout, err := u.detectManagedLayout()
	if err != nil {
		return nil, err
	}
	releaseInfo, err := u.fetchRelease(options.RequestedVersion)
	if err != nil {
		return nil, err
	}
	assetName, err := assetNameForPlatform(strings.TrimPrefix(releaseInfo.TagName, "v"), u.Runtime.GOOS, u.Runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	assets := make(map[string]string, len(releaseInfo.Assets))
	for _, asset := range releaseInfo.Assets {
		assets[asset.Name] = asset.URL
	}
	assetURL := assets[assetName]
	if assetURL == "" {
		return nil, fmt.Errorf("release %s does not contain asset %s", releaseInfo.TagName, assetName)
	}
	if err := u.validateReleaseAssetURL(assetURL, assetName); err != nil {
		return nil, err
	}
	checksumURL := assets[assetName+".sha256"]
	if checksumURL == "" {
		return nil, fmt.Errorf("release %s does not contain checksum asset %s", releaseInfo.TagName, assetName+".sha256")
	}
	if err := u.validateReleaseAssetURL(checksumURL, assetName+".sha256"); err != nil {
		return nil, err
	}
	keep := options.Keep
	if keep < 0 {
		keep = DefaultKeep
	}
	targetVersion := strings.TrimPrefix(releaseInfo.TagName, "v")
	return &plan{
		CurrentVersion: u.CurrentVersion,
		TargetVersion:  targetVersion,
		ReleaseTag:     ensureTagPrefix(releaseInfo.TagName),
		AssetName:      assetName,
		AssetURL:       assetURL,
		ChecksumURL:    checksumURL,
		InstallRoot:    layout.InstallRoot,
		BinDir:         layout.BinDir,
		CheckOnly:      options.CheckOnly,
		Force:          options.Force,
		Keep:           keep,
		AlreadyCurrent: targetVersion == u.CurrentVersion && !options.Force,
	}, nil
}
