package update

import (
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInstallScriptStagesBeforeReplacingTarget(t *testing.T) {
	root := t.TempDir()
	installRoot := filepath.Join(root, "install-root")
	binDir := filepath.Join(root, "bin")
	binaryName := "duplicacy-backup_9.9.9_linux_amd64"
	binaryPath := filepath.Join(root, "package", binaryName)
	targetPath := filepath.Join(installRoot, binaryName)

	writeFile(t, binaryPath, "new binary\n", 0755)
	writeFile(t, targetPath, "old binary\n", 0755)

	output := runSynologyInstallScript(t,
		"--binary", binaryPath,
		"--install-root", installRoot,
		"--bin-dir", binDir,
		"--keep", "2",
	)

	assertFileBody(t, targetPath, "new binary\n")
	assertExecutable(t, targetPath)
	assertNoInstallTemps(t, installRoot, binaryName)

	currentTarget, err := os.Readlink(filepath.Join(installRoot, "current"))
	if err != nil {
		t.Fatalf("Readlink(current) error = %v", err)
	}
	if currentTarget != binaryName {
		t.Fatalf("current target = %q, want %q", currentTarget, binaryName)
	}
	stableTarget, err := os.Readlink(filepath.Join(binDir, "duplicacy-backup"))
	if err != nil {
		t.Fatalf("Readlink(stable) error = %v", err)
	}
	if stableTarget != filepath.Join(installRoot, "current") {
		t.Fatalf("stable target = %q, want %q", stableTarget, filepath.Join(installRoot, "current"))
	}
	if !strings.Contains(output, "Installed: "+targetPath) ||
		!strings.Contains(output, "Retention policy: keeping newest 2 installed binaries") {
		t.Fatalf("output = %q", output)
	}
}

func TestInstallScriptNoActivateStillStagesReplacement(t *testing.T) {
	root := t.TempDir()
	installRoot := filepath.Join(root, "install-root")
	binDir := filepath.Join(root, "bin")
	binaryName := "duplicacy-backup_9.9.9_linux_amd64"
	binaryPath := filepath.Join(root, "package", binaryName)
	targetPath := filepath.Join(installRoot, binaryName)

	writeFile(t, binaryPath, "new binary\n", 0755)
	writeFile(t, targetPath, "old binary\n", 0755)

	output := runSynologyInstallScript(t,
		"--binary", binaryPath,
		"--install-root", installRoot,
		"--bin-dir", binDir,
		"--no-activate",
	)

	assertFileBody(t, targetPath, "new binary\n")
	assertExecutable(t, targetPath)
	assertNoInstallTemps(t, installRoot, binaryName)

	if _, err := os.Lstat(filepath.Join(installRoot, "current")); !os.IsNotExist(err) {
		t.Fatalf("current link exists or unexpected error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(binDir, "duplicacy-backup")); !os.IsNotExist(err) {
		t.Fatalf("stable link exists or unexpected error: %v", err)
	}
	if !strings.Contains(output, "Installed without activation") {
		t.Fatalf("output = %q", output)
	}
}

func TestInstallScriptUsesStagedRenameForBinaryReplacement(t *testing.T) {
	body, err := os.ReadFile(installScriptPath(t))
	if err != nil {
		t.Fatalf("ReadFile(install script) error = %v", err)
	}
	text := string(body)
	if strings.Contains(text, `cp "$BINARY_PATH" "$TARGET_PATH"`) {
		t.Fatal("install script still copies directly to the final target path")
	}
	for _, want := range []string{
		`cp "$BINARY_PATH" "$TEMP_TARGET_PATH"`,
		`chmod 755 "$TEMP_TARGET_PATH"`,
		`mv -f "$TEMP_TARGET_PATH" "$TARGET_PATH"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("install script missing %q", want)
		}
	}
}

func TestInstallScriptCanReplaceRunningBinaryOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ETXTBSY replacement behaviour is Linux-specific")
	}
	sleepPath, err := osexec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	truePath, err := osexec.LookPath("true")
	if err != nil {
		t.Skipf("true not available: %v", err)
	}

	root := t.TempDir()
	installRoot := filepath.Join(root, "install-root")
	binDir := filepath.Join(root, "bin")
	binaryName := "duplicacy-backup_9.9.9_linux_amd64"
	binaryPath := filepath.Join(root, "package", binaryName)
	targetPath := filepath.Join(installRoot, binaryName)

	copyFile(t, truePath, binaryPath, 0755)
	copyFile(t, sleepPath, targetPath, 0755)

	running := osexec.Command(targetPath, "5")
	if err := running.Start(); err != nil {
		t.Fatalf("Start(%q) error = %v", targetPath, err)
	}
	defer func() {
		_ = running.Process.Kill()
		_ = running.Wait()
	}()
	time.Sleep(100 * time.Millisecond)

	output := runSynologyInstallScript(t,
		"--binary", binaryPath,
		"--install-root", installRoot,
		"--bin-dir", binDir,
		"--no-activate",
	)

	assertNoInstallTemps(t, installRoot, binaryName)
	if !strings.Contains(output, "Installed: "+targetPath) {
		t.Fatalf("output = %q", output)
	}
	replaced := osexec.Command(targetPath)
	if err := replaced.Run(); err != nil {
		t.Fatalf("replaced target did not execute like the staged true binary: %v", err)
	}
}

func runSynologyInstallScript(t *testing.T, args ...string) string {
	t.Helper()
	cmd := osexec.Command("sh", append([]string{installScriptPath(t)}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}
	return string(output)
}

func installScriptPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "scripts", "install-synology.sh")
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%q) error = %v", path, err)
	}
	return abs
}

func writeFile(t *testing.T, path string, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func copyFile(t *testing.T, src string, dst string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		t.Fatalf("OpenFile(%q) error = %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatalf("Copy(%q -> %q) error = %v", src, dst, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close(%q) error = %v", dst, err)
	}
	if err := os.Chmod(dst, mode); err != nil {
		t.Fatalf("Chmod(%q) error = %v", dst, err)
	}
}

func assertFileBody(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, got, want)
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("%q mode = %v, want executable bit", path, info.Mode())
	}
}

func assertNoInstallTemps(t *testing.T, installRoot string, binaryName string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(installRoot, "."+binaryName+".*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary install files remain: %v", matches)
	}
}
