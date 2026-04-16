package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

const (
	DefaultRepo = "phillipmcmahon/synology-duplicacy-backup"
	DefaultKeep = 2
)

var versionedBinaryPattern = regexp.MustCompile(`^duplicacy-backup_(.+)_linux_(amd64|arm64|armv7)$`)

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

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []releaseAsset `json:"assets"`
}

type managedLayout struct {
	ExecutablePath string
	ResolvedPath   string
	InstallRoot    string
	BinDir         string
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
		return renderReport(planned, "Update available", ""), nil
	}
	if err := u.confirmInstall(planned, req); err != nil {
		return "", err
	}

	stageDir, err := u.Runtime.MkdirTemp(u.Runtime.TempDir(), "duplicacy-backup-update-")
	if err != nil {
		return "", fmt.Errorf("failed to create update staging directory: %w", err)
	}
	defer func() { _ = u.Runtime.RemoveAll(stageDir) }()

	tarballPath := filepath.Join(stageDir, planned.AssetName)
	checksumPath := filepath.Join(stageDir, planned.AssetName+".sha256")
	if err := u.downloadFile(planned.AssetURL, tarballPath); err != nil {
		return "", err
	}
	if err := u.downloadFile(planned.ChecksumURL, checksumPath); err != nil {
		return "", err
	}
	if err := verifyChecksum(tarballPath, checksumPath); err != nil {
		return "", err
	}
	extractDir := filepath.Join(stageDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create extraction directory: %w", err)
	}
	if err := extractTarball(tarballPath, extractDir); err != nil {
		return "", err
	}
	binaryPath, err := findVersionedBinary(extractDir)
	if err != nil {
		return "", err
	}
	scriptPath := filepath.Join(filepath.Dir(binaryPath), "install.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("update package is missing install.sh: %w", err)
	}

	args := []string{
		"--binary", binaryPath,
		"--install-root", planned.InstallRoot,
		"--bin-dir", planned.BinDir,
		"--keep", fmt.Sprintf("%d", planned.Keep),
	}
	output, err := u.RunInstaller(scriptPath, args)
	if err != nil {
		return "", fmt.Errorf("update install failed: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return renderReport(planned, "Installed", strings.TrimSpace(string(output))), nil
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
		Keep:           keep,
		AlreadyCurrent: targetVersion == u.CurrentVersion,
	}, nil
}

func (u *Updater) detectManagedLayout() (*managedLayout, error) {
	executablePath, err := u.Runtime.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to determine current executable path: %w", err)
	}
	resolvedPath, err := u.Runtime.EvalSymlinks(executablePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve current executable path: %w", err)
	}
	stablePath := executablePath
	if filepath.Base(stablePath) != u.ScriptName {
		commandPath, err := u.stableCommandPath(resolvedPath)
		if err != nil {
			return nil, err
		}
		stablePath = commandPath
	}
	if !versionedBinaryPattern.MatchString(filepath.Base(resolvedPath)) {
		return nil, fmt.Errorf("update only supports the managed install layout; resolved executable is %s", resolvedPath)
	}
	return &managedLayout{
		ExecutablePath: stablePath,
		ResolvedPath:   resolvedPath,
		InstallRoot:    filepath.Dir(resolvedPath),
		BinDir:         filepath.Dir(stablePath),
	}, nil
}

func (u *Updater) stableCommandPath(resolvedExecutable string) (string, error) {
	if u.Runtime.CommandPath == nil {
		return "", fmt.Errorf("update requires the managed stable command path %q; current executable is %s", u.ScriptName, resolvedExecutable)
	}
	commandPath := u.Runtime.CommandPath()
	if commandPath == "" || filepath.Base(commandPath) != u.ScriptName {
		return "", fmt.Errorf("update requires the managed stable command path %q; current executable is %s", u.ScriptName, resolvedExecutable)
	}
	if !filepath.IsAbs(commandPath) {
		if !strings.ContainsAny(commandPath, `/\`) {
			resolvedCommand, err := u.Runtime.LookPath(commandPath)
			if err != nil {
				return "", fmt.Errorf("failed to find invoked command %q on PATH: %w", commandPath, err)
			}
			commandPath = resolvedCommand
		} else {
			abs, err := filepath.Abs(commandPath)
			if err != nil {
				return "", fmt.Errorf("failed to resolve invoked command path %s: %w", commandPath, err)
			}
			commandPath = abs
		}
	}
	commandResolved, err := u.Runtime.EvalSymlinks(commandPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve invoked command path %s: %w", commandPath, err)
	}
	if commandResolved != resolvedExecutable {
		return "", fmt.Errorf("update requires the stable command path to resolve to the running binary; %s resolves to %s, running binary is %s", commandPath, commandResolved, resolvedExecutable)
	}
	return commandPath, nil
}

func (u *Updater) fetchRelease(requestedVersion string) (*release, error) {
	var url string
	if requestedVersion == "" {
		url = fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(u.APIBase, "/"), u.Repo)
	} else {
		url = fmt.Sprintf("%s/repos/%s/releases/tags/%s", strings.TrimRight(u.APIBase, "/"), u.Repo, ensureTagPrefix(requestedVersion))
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build GitHub release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", u.ScriptName)
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub release query failed: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}
	var parsed release
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub release metadata: %w", err)
	}
	if parsed.TagName == "" {
		return nil, errors.New("GitHub release metadata did not include a tag name")
	}
	return &parsed, nil
}

func (u *Updater) confirmInstall(planned *plan, req *workflow.Request) error {
	if req.UpdateYes {
		return nil
	}
	if !u.Runtime.StdinIsTTY() {
		return errors.New("update install requires --yes when not attached to a terminal; use --check-only to inspect the plan first")
	}
	fmt.Print(renderReport(planned, "Ready to install", ""))
	fmt.Print("Proceed with download and install? [y/N]: ")
	reader := bufio.NewReader(u.Runtime.Stdin())
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read update confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return errors.New("update cancelled at the interactive confirmation prompt")
	}
	return nil
}

func (u *Updater) downloadFile(url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build download request: %w", err)
	}
	req.Header.Set("User-Agent", u.ScriptName)
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", filepath.Base(path), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("failed to download %s: %s (%s)", filepath.Base(path), resp.Status, strings.TrimSpace(string(body)))
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

func assetNameForPlatform(version, goos, goarch string) (string, error) {
	if goos != "linux" {
		return "", fmt.Errorf("update only supports packaged Linux releases; current platform is %s/%s", goos, goarch)
	}
	switch goarch {
	case "amd64":
		return fmt.Sprintf("duplicacy-backup_%s_linux_amd64.tar.gz", version), nil
	case "arm64":
		return fmt.Sprintf("duplicacy-backup_%s_linux_arm64.tar.gz", version), nil
	case "arm":
		return fmt.Sprintf("duplicacy-backup_%s_linux_armv7.tar.gz", version), nil
	default:
		return "", fmt.Errorf("update does not support platform linux/%s", goarch)
	}
}

func ensureTagPrefix(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return version
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func verifyChecksum(tarballPath, checksumPath string) error {
	checksumBytes, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}
	fields := strings.Fields(string(checksumBytes))
	if len(fields) < 1 {
		return errors.New("checksum file does not contain a SHA256 value")
	}
	expected := fields[0]
	file, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded package: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to hash downloaded package: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("downloaded package checksum did not match %s", filepath.Base(checksumPath))
	}
	return nil
}

func extractTarball(tarballPath, destination string) error {
	file, err := os.Open(tarballPath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded package: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to read package gzip stream: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read package tar stream: %w", err)
		}
		name := filepath.Clean(header.Name)
		if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || filepath.IsAbs(name) {
			return fmt.Errorf("unsupported path in update package: %s", header.Name)
		}
		target := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create package directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create package parent directory %s: %w", filepath.Dir(target), err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create extracted file %s: %w", target, err)
			}
			if _, err := io.Copy(out, reader); err != nil {
				out.Close()
				return fmt.Errorf("failed to extract %s: %w", target, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("failed to close extracted file %s: %w", target, err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create package parent directory %s: %w", filepath.Dir(target), err)
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create extracted symlink %s: %w", target, err)
			}
		default:
			return fmt.Errorf("unsupported file in update package: %s", header.Name)
		}
	}
}

func findVersionedBinary(dir string) (string, error) {
	var found string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if versionedBinaryPattern.MatchString(entry.Name()) {
			if found != "" {
				return fmt.Errorf("extracted package contains multiple versioned duplicacy-backup binaries")
			}
			found = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to inspect extracted package: %w", err)
	}
	if found == "" {
		return "", errors.New("extracted package does not contain a versioned duplicacy-backup binary")
	}
	return found, nil
}

func runInstallScript(scriptPath string, args []string) ([]byte, error) {
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("sh", cmdArgs...)
	return cmd.CombinedOutput()
}

func renderReport(planned *plan, result string, installerOutput string) string {
	var b strings.Builder
	b.WriteString("Update\n")
	fmt.Fprintf(&b, "  Current Version      : %s\n", formatVersion(planned.CurrentVersion))
	fmt.Fprintf(&b, "  Target Version       : %s\n", planned.ReleaseTag)
	fmt.Fprintf(&b, "  Asset                : %s\n", planned.AssetName)
	fmt.Fprintf(&b, "  Install Root         : %s\n", planned.InstallRoot)
	fmt.Fprintf(&b, "  Bin Dir              : %s\n", planned.BinDir)
	fmt.Fprintf(&b, "  Keep                 : %d\n", planned.Keep)
	fmt.Fprintf(&b, "  Check Only           : %t\n", planned.CheckOnly)
	fmt.Fprintf(&b, "  Result               : %s\n", result)
	if installerOutput != "" {
		b.WriteString("  Section: Installer\n")
		scanner := bufio.NewScanner(strings.NewReader(installerOutput))
		for scanner.Scan() {
			fmt.Fprintf(&b, "    %s\n", scanner.Text())
		}
	}
	return b.String()
}

func formatVersion(version string) string {
	if version == "" {
		return "<unknown>"
	}
	return ensureTagPrefix(version)
}
