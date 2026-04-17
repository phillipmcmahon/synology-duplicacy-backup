package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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
	"time"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func githubReleaseAssetURL(version, assetName string) string {
	return "https://github.com/phillipmcmahon/synology-duplicacy-backup/releases/download/v" + version + "/" + assetName
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

func TestDefaultRuntimeAndNewFillDefaults(t *testing.T) {
	rt := DefaultRuntime()
	if rt.GOOS == "" || rt.GOARCH == "" {
		t.Fatalf("DefaultRuntime platform = %q/%q, want populated values", rt.GOOS, rt.GOARCH)
	}
	if rt.Stdin == nil || rt.StdinIsTTY == nil || rt.CommandPath == nil || rt.LookPath == nil ||
		rt.Executable == nil || rt.EvalSymlinks == nil || rt.TempDir == nil ||
		rt.MkdirTemp == nil || rt.RemoveAll == nil {
		t.Fatalf("DefaultRuntime did not populate all runtime hooks: %#v", rt)
	}

	updater := New("duplicacy-backup", "v4.3.1", Runtime{})
	if updater.Repo != DefaultRepo {
		t.Fatalf("Repo = %q, want %q", updater.Repo, DefaultRepo)
	}
	if updater.APIBase != "https://api.github.com" {
		t.Fatalf("APIBase = %q", updater.APIBase)
	}
	if updater.CurrentVersion != "4.3.1" {
		t.Fatalf("CurrentVersion = %q, want trimmed semver", updater.CurrentVersion)
	}
	if updater.HTTPClient != http.DefaultClient {
		t.Fatalf("HTTPClient = %#v, want http.DefaultClient", updater.HTTPClient)
	}
	if updater.ReleaseTimeout != DefaultReleaseMetadataTimeout {
		t.Fatalf("ReleaseTimeout = %s, want %s", updater.ReleaseTimeout, DefaultReleaseMetadataTimeout)
	}
	if updater.DownloadTimeout != DefaultAssetDownloadTimeout {
		t.Fatalf("DownloadTimeout = %s, want %s", updater.DownloadTimeout, DefaultAssetDownloadTimeout)
	}
	if updater.RunInstaller == nil {
		t.Fatal("RunInstaller was not populated")
	}
	if updater.Runtime.Stdin == nil || updater.Runtime.StdinIsTTY == nil ||
		updater.Runtime.CommandPath == nil || updater.Runtime.LookPath == nil ||
		updater.Runtime.Executable == nil || updater.Runtime.EvalSymlinks == nil ||
		updater.Runtime.TempDir == nil || updater.Runtime.MkdirTemp == nil ||
		updater.Runtime.RemoveAll == nil {
		t.Fatalf("New did not populate all runtime hooks: %#v", updater.Runtime)
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
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz")},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256")},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.Repo = "phillipmcmahon/synology-duplicacy-backup"
	updater.APIBase = server.URL

	result, err := updater.RunResult(Options{CheckOnly: true, Keep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusAvailable {
		t.Fatalf("Status = %q, want %q", result.Status, StatusAvailable)
	}
	output := result.Output
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
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz")},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256")},
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

	output, err := updater.Run(Options{CheckOnly: true, Keep: DefaultKeep})
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
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz")},
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz.sha256")},
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

	output, err := updater.Run(Options{CheckOnly: true, Keep: DefaultKeep})
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

	result, err := updater.RunResult(Options{Yes: true, Keep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusInstalled {
		t.Fatalf("Status = %q, want %q", result.Status, StatusInstalled)
	}
	output := result.Output
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
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz", URL: githubReleaseAssetURL("4.1.8", "duplicacy-backup_4.1.8_linux_amd64.tar.gz")},
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256", URL: githubReleaseAssetURL("4.1.8", "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256")},
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

	result, err := updater.RunResult(Options{Yes: true, Keep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusCurrent {
		t.Fatalf("Status = %q, want %q", result.Status, StatusCurrent)
	}
	output := result.Output
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

	output, err := updater.Run(Options{Yes: true, Force: true, Keep: DefaultKeep})
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
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz", URL: githubReleaseAssetURL("4.1.8", "duplicacy-backup_4.1.8_linux_amd64.tar.gz")},
				{Name: "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256", URL: githubReleaseAssetURL("4.1.8", "duplicacy-backup_4.1.8_linux_amd64.tar.gz.sha256")},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	result, err := updater.RunResult(Options{CheckOnly: true, Force: true, Keep: DefaultKeep})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusReinstallRequested {
		t.Fatalf("Status = %q, want %q", result.Status, StatusReinstallRequested)
	}
	output := result.Output
	if !strings.Contains(output, "Result               : Reinstall requested") ||
		!strings.Contains(output, "Force                : true") {
		t.Fatalf("output = %q", output)
	}
}

func TestConfirmInstallInteractiveResponses(t *testing.T) {
	planned := &plan{
		CurrentVersion: "4.1.8",
		TargetVersion:  "4.1.9",
		ReleaseTag:     "v4.1.9",
		AssetName:      "duplicacy-backup_4.1.9_linux_amd64.tar.gz",
		InstallRoot:    "/usr/local/lib/duplicacy-backup",
		BinDir:         "/usr/local/bin",
		Keep:           DefaultKeep,
	}

	yesInput := filepath.Join(t.TempDir(), "yes")
	if err := os.WriteFile(yesInput, []byte("yes\n"), 0644); err != nil {
		t.Fatalf("WriteFile(yesInput) failed: %v", err)
	}
	yesFile, err := os.Open(yesInput)
	if err != nil {
		t.Fatalf("Open(yesInput) failed: %v", err)
	}
	defer yesFile.Close()
	updater := New("duplicacy-backup", "4.1.8", Runtime{
		Stdin:      func() *os.File { return yesFile },
		StdinIsTTY: func() bool { return true },
	})
	if err := updater.confirmInstall(planned, Options{}); err != nil {
		t.Fatalf("confirmInstall(yes) error = %v", err)
	}

	noInput := filepath.Join(t.TempDir(), "no")
	if err := os.WriteFile(noInput, []byte("no\n"), 0644); err != nil {
		t.Fatalf("WriteFile(noInput) failed: %v", err)
	}
	noFile, err := os.Open(noInput)
	if err != nil {
		t.Fatalf("Open(noInput) failed: %v", err)
	}
	defer noFile.Close()
	updater = New("duplicacy-backup", "4.1.8", Runtime{
		Stdin:      func() *os.File { return noFile },
		StdinIsTTY: func() bool { return true },
	})
	err = updater.confirmInstall(planned, Options{})
	if err == nil || !strings.Contains(err.Error(), "update cancelled") {
		t.Fatalf("confirmInstall(no) err = %v", err)
	}

	updater = New("duplicacy-backup", "4.1.8", Runtime{
		StdinIsTTY: func() bool {
			t.Fatal("StdinIsTTY should not be checked when --yes is set")
			return false
		},
	})
	if err := updater.confirmInstall(planned, Options{Yes: true}); err != nil {
		t.Fatalf("confirmInstall(--yes) error = %v", err)
	}
}

func TestFetchReleaseErrorsAndRequestedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/phillipmcmahon/synology-duplicacy-backup/releases/tags/v4.1.9":
			_ = json.NewEncoder(w).Encode(release{TagName: "v4.1.9", Name: "v4.1.9"})
		case "/repos/phillipmcmahon/synology-duplicacy-backup/releases/latest":
			_ = json.NewEncoder(w).Encode(release{Name: "missing tag"})
		case "/repos/phillipmcmahon/synology-duplicacy-backup/releases/tags/vbad-json":
			_, _ = w.Write([]byte("{"))
		default:
			http.Error(w, "release not found", http.StatusTeapot)
		}
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", Runtime{})
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	got, err := updater.fetchRelease("4.1.9")
	if err != nil {
		t.Fatalf("fetchRelease(requested) error = %v", err)
	}
	if got.TagName != "v4.1.9" {
		t.Fatalf("TagName = %q, want v4.1.9", got.TagName)
	}

	_, err = updater.fetchRelease("")
	if err == nil || !strings.Contains(err.Error(), "did not include a tag name") {
		t.Fatalf("fetchRelease(latest missing tag) err = %v", err)
	}
	_, err = updater.fetchRelease("bad-json")
	if err == nil || !strings.Contains(err.Error(), "failed to decode") {
		t.Fatalf("fetchRelease(bad json) err = %v", err)
	}
	_, err = updater.fetchRelease("missing")
	if err == nil || !strings.Contains(err.Error(), "418") {
		t.Fatalf("fetchRelease(non-200) err = %v", err)
	}
}

func TestFetchReleaseAppliesTimeoutAndReportsOperatorMessage(t *testing.T) {
	updater := New("duplicacy-backup", "4.1.8", Runtime{})
	updater.ReleaseTimeout = 2 * time.Second
	var sawDeadline bool
	updater.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if _, ok := req.Context().Deadline(); ok {
				sawDeadline = true
			}
			return nil, context.DeadlineExceeded
		}),
	}

	_, err := updater.fetchRelease("")
	if err == nil || !strings.Contains(err.Error(), "GitHub release metadata request timed out after 2s") {
		t.Fatalf("fetchRelease(timeout) err = %v", err)
	}
	if !sawDeadline {
		t.Fatal("fetchRelease() request did not carry a context deadline")
	}
}

func TestDownloadFileWritesAndReportsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payload":
			_, _ = w.Write([]byte("payload"))
		default:
			http.Error(w, "missing", http.StatusNotFound)
		}
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", Runtime{})
	updater.HTTPClient = server.Client()

	outputPath := filepath.Join(t.TempDir(), "asset")
	if err := updater.downloadFile(server.URL+"/payload", outputPath); err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(outputPath) failed: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("downloaded payload = %q", got)
	}

	err = updater.downloadFile(server.URL+"/missing", filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("downloadFile(non-200) err = %v", err)
	}

	err = updater.downloadFile(server.URL+"/payload", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "failed to create") {
		t.Fatalf("downloadFile(create failure) err = %v", err)
	}
}

func TestDownloadFileValidatesRedirectHosts(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect-ok":
			http.Redirect(w, r, server.URL+"/payload", http.StatusFound)
		case "/redirect-bad":
			http.Redirect(w, r, "https://evil.example/payload", http.StatusFound)
		case "/payload":
			_, _ = w.Write([]byte("payload"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", Runtime{})
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL

	outputPath := filepath.Join(t.TempDir(), "asset")
	if err := updater.downloadFile(server.URL+"/redirect-ok", outputPath); err != nil {
		t.Fatalf("downloadFile(allowed redirect) error = %v", err)
	}

	err := updater.downloadFile(server.URL+"/redirect-bad", filepath.Join(t.TempDir(), "asset.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "unexpected host \"evil.example\"") {
		t.Fatalf("downloadFile(rejected redirect) err = %v", err)
	}
}

func TestDownloadFileAppliesTimeoutAndReportsOperatorMessage(t *testing.T) {
	updater := New("duplicacy-backup", "4.1.8", Runtime{})
	updater.DownloadTimeout = 3 * time.Second
	var sawDeadline bool
	updater.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if _, ok := req.Context().Deadline(); ok {
				sawDeadline = true
			}
			return nil, context.DeadlineExceeded
		}),
	}

	err := updater.downloadFile("https://example.invalid/asset.tar.gz", filepath.Join(t.TempDir(), "asset.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "download timed out after 3s while downloading asset.tar.gz") {
		t.Fatalf("downloadFile(timeout) err = %v", err)
	}
	if !sawDeadline {
		t.Fatal("downloadFile() request did not carry a context deadline")
	}
}

func TestBuildPlanRejectsUnexpectedAssetURLs(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	assetName := "duplicacy-backup_4.1.9_linux_amd64.tar.gz"
	checksumName := assetName + ".sha256"

	cases := []struct {
		name        string
		assetURL    string
		checksumURL string
		want        string
	}{
		{
			name:        "tarball host",
			assetURL:    "https://evil.example/" + assetName,
			checksumURL: githubReleaseAssetURL("4.1.9", checksumName),
			want:        "unexpected host \"evil.example\"",
		},
		{
			name:        "checksum name",
			assetURL:    githubReleaseAssetURL("4.1.9", assetName),
			checksumURL: githubReleaseAssetURL("4.1.9", "wrong.sha256"),
			want:        "does not end with the expected asset name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(release{
					TagName: "v4.1.9",
					Name:    "v4.1.9",
					Assets: []releaseAsset{
						{Name: assetName, URL: tc.assetURL},
						{Name: checksumName, URL: tc.checksumURL},
					},
				})
			}))
			defer server.Close()

			updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
			updater.HTTPClient = server.Client()
			updater.APIBase = server.URL

			_, err := updater.buildPlan(Options{CheckOnly: true, Keep: DefaultKeep})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("buildPlan() err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestVerifyChecksumFailures(t *testing.T) {
	dir := t.TempDir()
	tarballPath := filepath.Join(dir, "asset.tar.gz")
	if err := os.WriteFile(tarballPath, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFile(tarballPath) failed: %v", err)
	}

	checksumPath := filepath.Join(dir, "asset.tar.gz.sha256")
	if err := os.WriteFile(checksumPath, nil, 0644); err != nil {
		t.Fatalf("WriteFile(empty checksum) failed: %v", err)
	}
	err := verifyChecksum(tarballPath, checksumPath)
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("verifyChecksum(empty) err = %v", err)
	}

	if err := os.WriteFile(checksumPath, []byte(strings.Repeat("0", 64)+"  asset.tar.gz\n"), 0644); err != nil {
		t.Fatalf("WriteFile(mismatch checksum) failed: %v", err)
	}
	err = verifyChecksum(tarballPath, checksumPath)
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("verifyChecksum(mismatch) err = %v", err)
	}

	sum := sha256.Sum256([]byte("payload"))
	if err := os.WriteFile(checksumPath, []byte(hex.EncodeToString(sum[:])+"  asset.tar.gz\n"), 0644); err != nil {
		t.Fatalf("WriteFile(valid checksum) failed: %v", err)
	}
	err = verifyChecksum(filepath.Join(dir, "missing.tar.gz"), checksumPath)
	if err == nil || !strings.Contains(err.Error(), "failed to open") {
		t.Fatalf("verifyChecksum(missing tarball) err = %v", err)
	}
}

func TestExtractTarballHandlesDirsFilesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	tarballPath := filepath.Join(dir, "package.tar.gz")
	writeTarGz(t, tarballPath, []tarEntry{
		{name: "package/", typ: tar.TypeDir, mode: 0755},
		{name: "package/bin/", typ: tar.TypeDir, mode: 0755},
		{name: "package/bin/duplicacy-backup_4.1.9_linux_amd64", typ: tar.TypeReg, mode: 0755, body: "#!/bin/sh\n"},
		{name: "package/current", typ: tar.TypeSymlink, mode: 0777, linkname: "bin/duplicacy-backup_4.1.9_linux_amd64"},
	})

	destination := filepath.Join(dir, "out")
	if err := extractTarball(tarballPath, destination); err != nil {
		t.Fatalf("extractTarball() error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(destination, "package/bin/duplicacy-backup_4.1.9_linux_amd64"))
	if err != nil {
		t.Fatalf("ReadFile(extracted binary) failed: %v", err)
	}
	if string(body) != "#!/bin/sh\n" {
		t.Fatalf("extracted body = %q", body)
	}
	linkTarget, err := os.Readlink(filepath.Join(destination, "package/current"))
	if err != nil {
		t.Fatalf("Readlink(extracted symlink) failed: %v", err)
	}
	if linkTarget != "bin/duplicacy-backup_4.1.9_linux_amd64" {
		t.Fatalf("symlink target = %q", linkTarget)
	}
}

func TestExtractTarballMasksArchiveModeBits(t *testing.T) {
	dir := t.TempDir()
	tarballPath := filepath.Join(dir, "package.tar.gz")
	writeTarGz(t, tarballPath, []tarEntry{
		{name: "package/", typ: tar.TypeDir, mode: 04777},
		{name: "package/bin", typ: tar.TypeReg, mode: 04755, body: "#!/bin/sh\n"},
	})

	destination := filepath.Join(dir, "out")
	if err := extractTarball(tarballPath, destination); err != nil {
		t.Fatalf("extractTarball() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(destination, "package"),
		filepath.Join(destination, "package/bin"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%s) failed: %v", path, err)
		}
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			t.Fatalf("%s mode = %v, want no special permission bits", path, info.Mode())
		}
	}
}

func TestArchivePermMasksSpecialBits(t *testing.T) {
	tests := []struct {
		mode int64
		want os.FileMode
	}{
		{mode: 04755, want: 0755},
		{mode: 02770, want: 0770},
		{mode: 01777, want: 0777},
	}

	for _, tt := range tests {
		if got := archivePerm(tt.mode); got != tt.want {
			t.Fatalf("archivePerm(%#o) = %#o, want %#o", tt.mode, got, tt.want)
		}
	}
}

func TestExtractTarballRejectsUnsafeAndUnsupportedEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []tarEntry
		wantErr string
	}{
		{
			name:    "parent traversal",
			entries: []tarEntry{{name: "../escape", typ: tar.TypeReg, mode: 0644, body: "nope"}},
			wantErr: "unsupported path",
		},
		{
			name:    "absolute path",
			entries: []tarEntry{{name: "/tmp/escape", typ: tar.TypeReg, mode: 0644, body: "nope"}},
			wantErr: "unsupported path",
		},
		{
			name:    "hard link",
			entries: []tarEntry{{name: "package/link", typ: tar.TypeLink, mode: 0644, linkname: "target"}},
			wantErr: "unsupported file",
		},
		{
			name:    "symlink parent escape",
			entries: []tarEntry{{name: "package/current", typ: tar.TypeSymlink, mode: 0777, linkname: "../../etc/passwd"}},
			wantErr: "unsupported symlink target",
		},
		{
			name:    "symlink absolute escape",
			entries: []tarEntry{{name: "package/current", typ: tar.TypeSymlink, mode: 0777, linkname: "/etc/passwd"}},
			wantErr: "unsupported symlink target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tarballPath := filepath.Join(dir, "package.tar.gz")
			writeTarGz(t, tarballPath, tt.entries)

			err := extractTarball(tarballPath, filepath.Join(dir, "out"))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("extractTarball() err = %v, want %q", err, tt.wantErr)
			}
		})
	}

	plainFile := filepath.Join(t.TempDir(), "not-gzip.tar.gz")
	if err := os.WriteFile(plainFile, []byte("not gzip"), 0644); err != nil {
		t.Fatalf("WriteFile(plainFile) failed: %v", err)
	}
	err := extractTarball(plainFile, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "gzip stream") {
		t.Fatalf("extractTarball(non-gzip) err = %v", err)
	}
}

func TestFindVersionedBinaryErrors(t *testing.T) {
	_, err := findVersionedBinary(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("findVersionedBinary(empty) err = %v", err)
	}

	dir := t.TempDir()
	for _, name := range []string{
		"duplicacy-backup_4.1.8_linux_amd64",
		"duplicacy-backup_4.1.9_linux_amd64",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("WriteFile(%s) failed: %v", name, err)
		}
	}
	_, err = findVersionedBinary(dir)
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("findVersionedBinary(multiple) err = %v", err)
	}
}

func TestBuildPlanRejectsMissingAssetsAndUnsupportedPlatforms(t *testing.T) {
	executablePath, _ := managedExecutableLayout(t, "4.1.8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(release{
			TagName: "v4.1.9",
			Name:    "v4.1.9",
			Assets: []releaseAsset{
				{Name: "duplicacy-backup_4.1.9_linux_amd64.tar.gz", URL: githubReleaseAssetURL("4.1.9", "duplicacy-backup_4.1.9_linux_amd64.tar.gz")},
			},
		})
	}))
	defer server.Close()

	updater := New("duplicacy-backup", "4.1.8", testRuntime(executablePath))
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL
	_, err := updater.buildPlan(Options{CheckOnly: true, Keep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "does not contain checksum asset") {
		t.Fatalf("buildPlan(missing checksum) err = %v", err)
	}

	rt := testRuntime(executablePath)
	rt.GOOS = "darwin"
	updater = New("duplicacy-backup", "4.1.8", rt)
	updater.HTTPClient = server.Client()
	updater.APIBase = server.URL
	_, err = updater.buildPlan(Options{CheckOnly: true, Keep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "only supports packaged Linux releases") {
		t.Fatalf("buildPlan(unsupported platform) err = %v", err)
	}
}

func TestRunInstallScriptDoesNotInheritMissingWorkingDirectory(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "install.sh")
	script := "#!/bin/sh\nset -eu\nprintf 'installer-pwd=%s\\n' \"$(pwd)\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile(scriptPath) failed: %v", err)
	}

	originalWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWorkingDir); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	})
	missingWorkingDir := filepath.Join(t.TempDir(), "removed")
	if err := os.Mkdir(missingWorkingDir, 0755); err != nil {
		t.Fatalf("Mkdir(missingWorkingDir) failed: %v", err)
	}
	if err := os.Chdir(missingWorkingDir); err != nil {
		t.Fatalf("Chdir(missingWorkingDir) failed: %v", err)
	}
	if err := os.RemoveAll(missingWorkingDir); err != nil {
		t.Fatalf("RemoveAll(missingWorkingDir) failed: %v", err)
	}

	output, err := runInstallScript(scriptPath, nil)
	if err != nil {
		t.Fatalf("runInstallScript() error = %v\n%s", err, output)
	}
	if strings.Contains(string(output), "getcwd") {
		t.Fatalf("runInstallScript() inherited missing working directory warnings: %s", output)
	}
	want := "installer-pwd=" + scriptDir
	if !strings.Contains(string(output), want) {
		t.Fatalf("runInstallScript() output = %q, want %q", output, want)
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

	_, err := updater.Run(Options{Keep: DefaultKeep})
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

	_, err := updater.Run(Options{CheckOnly: true, Keep: DefaultKeep})
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
	_, err := updater.Run(Options{CheckOnly: true, Keep: DefaultKeep})
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
	_, err := updater.Run(Options{CheckOnly: true, Keep: DefaultKeep})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve invoked command path") {
		t.Fatalf("Run() err = %v", err)
	}
}

func buildPackageTarball(t *testing.T, version string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	prefix := "duplicacy-backup_" + version + "_linux_amd64/"
	writeTarGzToWriter(t, &compressed, []tarEntry{
		{name: prefix + "duplicacy-backup_" + version + "_linux_amd64", typ: tar.TypeReg, mode: 0755, body: "#!/bin/sh\n"},
		{name: prefix + "install.sh", typ: tar.TypeReg, mode: 0755, body: "#!/bin/sh\n"},
		{name: prefix + "README.md", typ: tar.TypeReg, mode: 0644, body: "test\n"},
	})
	return compressed.Bytes()
}

type tarEntry struct {
	name     string
	typ      byte
	mode     int64
	body     string
	linkname string
}

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	var compressed bytes.Buffer
	writeTarGzToWriter(t, &compressed, entries)
	if err := os.WriteFile(path, compressed.Bytes(), 0644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
}

func writeTarGzToWriter(t *testing.T, compressed *bytes.Buffer, entries []tarEntry) {
	t.Helper()
	gz := gzip.NewWriter(compressed)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		size := int64(len(entry.body))
		if entry.typ != tar.TypeReg {
			size = 0
		}
		header := &tar.Header{
			Name:     entry.name,
			Typeflag: entry.typ,
			Mode:     entry.mode,
			Size:     size,
			Linkname: entry.linkname,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%s) failed: %v", entry.name, err)
		}
		if entry.typ == tar.TypeReg {
			if _, err := tw.Write([]byte(entry.body)); err != nil {
				t.Fatalf("Write(%s) failed: %v", entry.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close() failed: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip.Close() failed: %v", err)
	}
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
