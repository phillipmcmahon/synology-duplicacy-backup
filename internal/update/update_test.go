package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func managedExecutableLayout(t *testing.T, version string) (string, string) {
	t.Helper()
	root := t.TempDir()
	installRoot := filepath.Join(root, "install-root")
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(installRoot, 0755); err != nil {
		t.Fatalf("MkdirAll(installRoot) failed: %v", err)
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("MkdirAll(binDir) failed: %v", err)
	}
	versioned := filepath.Join(installRoot, "duplicacy-backup_"+version+"_linux_amd64")
	if err := os.WriteFile(versioned, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("WriteFile(versioned) failed: %v", err)
	}
	if err := os.Symlink(filepath.Base(versioned), filepath.Join(installRoot, "current")); err != nil {
		t.Fatalf("Symlink(current) failed: %v", err)
	}
	executablePath := filepath.Join(binDir, "duplicacy-backup")
	if err := os.Symlink(filepath.Join(installRoot, "current"), executablePath); err != nil {
		t.Fatalf("Symlink(stable) failed: %v", err)
	}
	return executablePath, versioned
}

func testRuntime(executablePath string) Runtime {
	return Runtime{
		GOOS:         "linux",
		GOARCH:       "amd64",
		Stdin:        func() *os.File { return os.Stdin },
		StdinIsTTY:   func() bool { return false },
		Executable:   func() (string, error) { return executablePath, nil },
		EvalSymlinks: filepath.EvalSymlinks,
		TempDir:      os.TempDir,
		MkdirTemp:    os.MkdirTemp,
		RemoveAll:    os.RemoveAll,
	}
}

func TestAssetNameForPlatform(t *testing.T) {
	tests := []struct {
		goos    string
		goarch  string
		want    string
		wantErr string
	}{
		{goos: "linux", goarch: "amd64", want: "duplicacy-backup_4.1.9_linux_amd64.tar.gz"},
		{goos: "linux", goarch: "arm64", want: "duplicacy-backup_4.1.9_linux_arm64.tar.gz"},
		{goos: "linux", goarch: "arm", want: "duplicacy-backup_4.1.9_linux_armv7.tar.gz"},
		{goos: "darwin", goarch: "amd64", wantErr: "update only supports packaged Linux releases"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got, err := assetNameForPlatform("4.1.9", tt.goos, tt.goarch)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("assetNameForPlatform() err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("assetNameForPlatform() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("assetNameForPlatform() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunCheckOnlyReportsAvailableUpdate(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/phillipmcmahon/synology-duplicacy-backup/releases/latest" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.9",
			Name:    "v4.1.9",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: "https://example.invalid/asset"},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: "https://example.invalid/asset.sha256"},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.Repo = "phillipmcmahon/synology-duplicacy-backup"
	updater.APIBase = server.URL

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Target Version       : v4.1.9") ||
		!strings.Contains(output, "Result               : Update available") ||
		!strings.Contains(output, "Keep                 : 2") {
		t.Fatalf("output = %q", output)
	}
}

func TestRunCheckOnlyUsesInvokedStablePathWhenExecutableIsResolved(t *testing.T) {
	executablePath, resolvedPath := managedExecutableLayout(t, "4.1.8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.9",
			Name:    "v4.1.9",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: "https://example.invalid/asset"},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: "https://example.invalid/asset.sha256"},
			},
		})
	}))
	defer server.Close()

	rt := testRuntime(executablePath)
	rt.Executable = func() (string, error) { return resolvedPath, nil }
	rt.CommandPath = func() string { return executablePath }
	updater := New("duplicacy-backup", "4.1.8", rt)
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Result               : Update available") ||
		!strings.Contains(output, "Bin Dir              : "+filepath.Dir(executablePath)) {
		t.Fatalf("output = %q", output)
	}
}

func TestRunCheckOnlyResolvesBareCommandThroughPath(t *testing.T) {
	executablePath, resolvedPath := managedExecutableLayout(t, "4.1.8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.9",
			Name:    "v4.1.9",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: "https://example.invalid/asset"},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: "https://example.invalid/asset.sha256"},
			},
		})
	}))
	defer server.Close()
	t.Chdir(t.TempDir())

	rt := testRuntime(executablePath)
	rt.Executable = func() (string, error) { return resolvedPath, nil }
	rt.CommandPath = func() string { return "duplicacy-backup" }
	rt.LookPath = func(name string) (string, error) {
		if name != "duplicacy-backup" {
			t.Fatalf("LookPath(%q), want duplicacy-backup", name)
		}
		return executablePath, nil
	}
	updater := New("duplicacy-backup", "4.1.8", rt)
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Result               : Update available") ||
		!strings.Contains(output, "Bin Dir              : "+filepath.Dir(executablePath)) {
		t.Fatalf("output = %q", output)
	}
}

func TestRunInstallDownloadsVerifiesAndRunsInstaller(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	tarball := buildPackageTarball(t, "4.1.9")
	sum := sha256.Sum256(tarball)
	checksum := hex.EncodeToString(sum[:]) + "  duplicacy-backup_4.1.9_linux_amd64.tar.gz\n"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/phillipmcmahon/synology-duplicacy-backup/releases/latest":
			_ = json.NewEncoder(w).Encode(release{
				TagName: "v4.1.9",
				Name:    "v4.1.9",
				Assets: []releaseAsset{
					{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: server.URL + "/asset"},
					{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: server.URL + "/asset.sha256"},
				},
			})
		case "/asset":
			_, _ = w.Write(tarball)
		case "/asset.sha256":
			_, _ = w.Write([]byte(checksum))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL
	var gotScript string
	var gotArgs []string
	updater.RunInstaller = func(scriptPath string, args []string) ([]byte, error) {
		gotScript = scriptPath
		gotArgs = append([]string(nil), args...)
		return []byte("Installed: ok\nActivated: ok"), nil
	}

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateYes: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Result               : Installed") ||
		!strings.Contains(output, "Installed: ok") {
		t.Fatalf("output = %q", output)
	}
	if filepath.Base(gotScript) != "install.sh" {
		t.Fatalf("gotScript = %q", gotScript)
	}
	if !containsArgPair(gotArgs, "--keep", "2") || !containsArg(gotArgs, "--binary") || !containsArg(gotArgs, "--install-root") || !containsArg(gotArgs, "--bin-dir") {
		t.Fatalf("gotArgs = %#v", gotArgs)
	}
}

func TestRunAlreadyCurrentSkipsInstallWithoutForce(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.8",
			Name:    "v4.1.8",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz", URL: "https://example.invalid/asset"},
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256", URL: "https://example.invalid/asset.sha256"},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL
	updater.RunInstaller = func(string, []string) ([]byte, error) {
		t.Fatal("RunInstaller should not be called when already current")
		return nil, nil
	}

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateYes: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Result               : Already up to date") ||
		!strings.Contains(output, "Force                : false") {
		t.Fatalf("output = %q", output)
	}
}

func TestRunForceReinstallsAlreadyCurrentVersion(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	tarball := buildPackageTarball(t, "4.1.8")
	sum := sha256.Sum256(tarball)
	checksum := hex.EncodeToString(sum[:]) + "  duplicacy-backup_4.1.8_linux_amd64.tar.gz\n"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/phillipmcmahon/synology-duplicacy-backup/releases/latest":
			_ = json.NewEncoder(w).Encode(release{
				TagName: "v4.1.8",
				Name:    "v4.1.8",
				Assets: []releaseAsset{
					{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz", URL: server.URL + "/asset"},
					{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256", URL: server.URL + "/asset.sha256"},
				},
			})
		case "/asset":
			_, _ = w.Write(tarball)
		case "/asset.sha256":
			_, _ = w.Write([]byte(checksum))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL
	installerCalled := false
	updater.RunInstaller = func(string, []string) ([]byte, error) {
		installerCalled = true
		return []byte("Installed: forced reinstall"), nil
	}

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateYes: true, UpdateForce: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !installerCalled {
		t.Fatal("RunInstaller was not called")
	}
	if !strings.Contains(output, "Result               : Installed") ||
		!strings.Contains(output, "Force                : true") ||
		!strings.Contains(output, "Installed: forced reinstall") {
		t.Fatalf("output = %q", output)
	}
}

func TestRunForceCheckOnlyReportsReinstall(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.8",
			Name:    "v4.1.8",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz", URL: "https://example.invalid/asset"},
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256", URL: "https://example.invalid/asset.sha256"},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	output, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateForce: true, UpdateKeep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Result               : Reinstall requested") ||
		!strings.Contains(output, "Force                : true") {
		t.Fatalf("output = %q", output)
	}
}

func TestRunInstallRequiresYesWithoutTTY(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.9",
			Name:    "v4.1.9",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: server.URL + "/asset"},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: server.URL + "/asset.sha256"},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	_, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateKeep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("Run() err = %v", err)
	}
}

func TestRunRejectsUnmanagedLayout(t *testing.T) {
	updater := New("duplicacy-backup", "4.1.8", Runtime{
		GOOS:         "linux",
		GOARCH:       "amd64",
		Stdin:        func() *os.File { return os.Stdin },
		StdinIsTTY:   func() bool { return false },
		Executable:   func() (string, error) { return "/tmp/custom-binary", nil },
		EvalSymlinks: func(path string) (string, error) { return path, nil },
		TempDir:      os.TempDir,
		MkdirTemp:    os.MkdirTemp,
		RemoveAll:    os.RemoveAll,
	})

	_, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateKeep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "managed stable command path") {
		t.Fatalf("Run() err = %v", err)
	}
}

func TestRunRejectsBareCommandMissingFromPath(t *testing.T) {
	executablePath, resolvedPath := managedExecutableLayout(t, "4.1.8")
	rt := testRuntime(executablePath)
	rt.Executable = func() (string, error) { return resolvedPath, nil }
	rt.CommandPath = func() string { return "duplicacy-backup" }
	rt.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	updater := New("duplicacy-backup", "4.1.8", rt)
	_, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateKeep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "failed to find invoked command \"duplicacy-backup\" on PATH") {
		t.Fatalf("Run() err = %v", err)
	}
}

func TestRunRejectsMissingRelativeCommandPath(t *testing.T) {
	executablePath, resolvedPath := managedExecutableLayout(t, "4.1.8")
	t.Chdir(t.TempDir())
	rt := testRuntime(executablePath)
	rt.Executable = func() (string, error) { return resolvedPath, nil }
	rt.CommandPath = func() string { return "./duplicacy-backup" }
	rt.LookPath = func(name string) (string, error) {
		t.Fatalf("LookPath(%q) should not be used for explicit relative paths", name)
		return "", exec.ErrNotFound
	}

	updater := New("duplicacy-backup", "4.1.8", rt)
	_, err := updater.Run(&workflow.Request{UpdateCommand: "update", UpdateCheckOnly: true, UpdateKeep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve invoked command path") {
		t.Fatalf("Run() err = %v", err)
	}
}

func buildPackageTarball(t *testing.T, version string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	prefix := "duplicacy-backup_" + version + "_linux_amd64/"

	files := []struct {
		name string
		mode int64
		body string
	}{
		{name: prefix + "duplicacy-backup_" + version + "_linux_amd64", mode: 0755, body: "#!/bin/sh\n"},
		{name: prefix + "install.sh", mode: 0755, body: "#!/bin/sh\n"},
		{name: prefix + "README.md", mode: 0644, body: "test\n"},
	}
	for _, file := range files {
		header := &tar.Header{
			Name: file.name,
			Mode: file.mode,
			Size: int64(len(file.body)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%s) failed: %v", file.name, err)
		}
		if _, err := tw.Write([]byte(file.body)); err != nil {
			t.Fatalf("Write(%s) failed: %v", file.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close() failed: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip.Close() failed: %v", err)
	}
	return compressed.Bytes()
}

func containsArg(args []string, key string) bool {
	for _, arg := range args {
		if arg == key {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
