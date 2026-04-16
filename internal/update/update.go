package update

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

const (
	DefaultRepo = "phillipmcmahon/synology-duplicacy-backup"
	DefaultKeep = 2
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
	Repo           string
	APIBase        string
	ScriptName     string
	CurrentVersion string
	Runtime        Runtime
	HTTPClient     *http.Client
	RunInstaller   func(string, []string) ([]byte, error)
}

func DefaultRuntime() Runtime {
	return Runtime{
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Stdin:        func() *os.File { return os.Stdin },
		StdinIsTTY:   func() bool { return workflow.DefaultRuntime().StdinIsTTY() },
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
		rt.StdinIsTTY = func() bool { return workflow.DefaultRuntime().StdinIsTTY() }
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
		Repo:           DefaultRepo,
		APIBase:        "https://api.github.com",
		ScriptName:     scriptName,
		CurrentVersion: strings.TrimPrefix(currentVersion, "v"),
		Runtime:        rt,
		HTTPClient:     http.DefaultClient,
		RunInstaller:   runInstallScript,
	}
}

func HandleCommand(req *workflow.Request, meta workflow.Metadata, rt workflow.Runtime) (string, error) {
	updater := New(meta.ScriptName, meta.Version, Runtime{
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Stdin:        rt.Stdin,
		StdinIsTTY:   rt.StdinIsTTY,
		CommandPath:  func() string { return os.Args[0] },
		LookPath:     exec.LookPath,
		Executable:   rt.Executable,
		EvalSymlinks: rt.EvalSymlinks,
		TempDir:      rt.TempDir,
		MkdirTemp:    os.MkdirTemp,
		RemoveAll:    os.RemoveAll,
	})
	return updater.Run(req)
}

func (u *Updater) Run(req *workflow.Request) (string, error) {
	planned, err := u.buildPlan(req)
	if err != nil {
		return "", err
	}
	if planned.AlreadyCurrent {
		return renderReport(planned, "Already up to date", ""), nil
	}
	if planned.CheckOnly {
		result := "Update available"
		if planned.Force && planned.TargetVersion == planned.CurrentVersion {
			result = "Reinstall requested"
		}
		return renderReport(planned, result, ""), nil
	}
	if err := u.confirmInstall(planned, req); err != nil {
		return "", err
	}
	installerOutput, err := u.install(planned)
	if err != nil {
		return "", err
	}
	return renderReport(planned, "Installed", installerOutput), nil
}

func (u *Updater) buildPlan(req *workflow.Request) (*plan, error) {
	layout, err := u.detectManagedLayout()
	if err != nil {
		return nil, err
	}
	releaseInfo, err := u.fetchRelease(req.UpdateVersion)
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
	checksumURL := assets[assetName+".sha256"]
	if checksumURL == "" {
		return nil, fmt.Errorf("release %s does not contain checksum asset %s", releaseInfo.TagName, assetName+".sha256")
	}
	keep := req.UpdateKeep
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
		CheckOnly:      req.UpdateCheckOnly,
		Force:          req.UpdateForce,
		Keep:           keep,
		AlreadyCurrent: targetVersion == u.CurrentVersion && !req.UpdateForce,
	}, nil
}
