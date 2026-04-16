package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []releaseAsset `json:"assets"`
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
