package update

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type managedLayout struct {
	ExecutablePath string
	ResolvedPath   string
	InstallRoot    string
	BinDir         string
}

func (u *Updater) confirmInstall(planned *plan, options Options) error {
	if options.Yes {
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

func (u *Updater) install(planned *plan) (string, error) {
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
	if err := u.verifyReleaseAssetAttestation(planned, tarballPath); err != nil {
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
	return strings.TrimSpace(string(output)), nil
}

// detectManagedLayout verifies the managed update invariant before any install
// work begins: operators invoke duplicacy-backup through a stable command on
// PATH, that command resolves through the managed symlink, and the symlink
// resolves to the versioned binary that is currently running.
func (u *Updater) detectManagedLayout() (*managedLayout, error) {
	return u.detectManagedLayoutFor("update")
}

func (u *Updater) detectManagedLayoutFor(operation string) (*managedLayout, error) {
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
		commandPath, err := u.stableCommandPathFor(operation, resolvedPath)
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
	return u.stableCommandPathFor("update", resolvedExecutable)
}

func (u *Updater) stableCommandPathFor(operation, resolvedExecutable string) (string, error) {
	if u.Runtime.CommandPath == nil {
		return "", fmt.Errorf("%s requires the managed stable command path %q; current executable is %s", operation, u.ScriptName, resolvedExecutable)
	}
	commandPath := u.Runtime.CommandPath()
	if commandPath == "" || filepath.Base(commandPath) != u.ScriptName {
		return "", fmt.Errorf("%s requires the managed stable command path %q; current executable is %s", operation, u.ScriptName, resolvedExecutable)
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

func runInstallScript(scriptPath string, args []string) ([]byte, error) {
	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("sh", cmdArgs...)
	cmd.Dir = filepath.Dir(scriptPath)
	return cmd.CombinedOutput()
}
