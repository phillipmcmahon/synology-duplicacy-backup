package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var versionedBinaryPattern = regexp.MustCompile(`^duplicacy-backup_(.+)_linux_(amd64|arm64|armv7)$`)

func (u *Updater) downloadFile(url, path string) error {
	timeout := requestTimeout(u.DownloadTimeout, DefaultAssetDownloadTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build download request: %w", err)
	}
	req.Header.Set("User-Agent", u.ScriptName)
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			return fmt.Errorf("download timed out after %s while downloading %s: %w", timeout, filepath.Base(path), err)
		}
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
		if isTimeoutError(err) {
			return fmt.Errorf("download timed out after %s while writing %s: %w", timeout, filepath.Base(path), err)
		}
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
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
