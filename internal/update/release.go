package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	urlpath "path"
	"strings"
	"time"
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
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout(u.ReleaseTimeout, DefaultReleaseMetadataTimeout))
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build GitHub release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", u.ScriptName)
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			return nil, fmt.Errorf("GitHub release metadata request timed out after %s: %w", requestTimeout(u.ReleaseTimeout, DefaultReleaseMetadataTimeout), err)
		}
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

func (u *Updater) validateReleaseAssetURL(rawURL, assetName string) error {
	parsed, err := parseDownloadURL(rawURL, assetName, "release asset URL")
	if err != nil {
		return err
	}
	if u.isSameCustomAPIBase(parsed) {
		return nil
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("release asset URL for %s must use https: %s", assetName, rawURL)
	}
	if !strings.EqualFold(parsed.Hostname(), "github.com") {
		return fmt.Errorf("release asset URL for %s uses unexpected host %q: %s", assetName, parsed.Hostname(), rawURL)
	}
	expectedPrefix := "/" + strings.Trim(u.Repo, "/") + "/releases/download/"
	if !strings.HasPrefix(strings.ToLower(parsed.Path), strings.ToLower(expectedPrefix)) {
		return fmt.Errorf("release asset URL for %s is outside expected GitHub release path %q: %s", assetName, expectedPrefix, rawURL)
	}
	if urlpath.Base(parsed.Path) != assetName {
		return fmt.Errorf("release asset URL for %s does not end with the expected asset name: %s", assetName, rawURL)
	}
	return nil
}

func (u *Updater) validateDownloadRedirectURL(rawURL, assetName string) error {
	parsed, err := parseDownloadURL(rawURL, assetName, "release asset redirect")
	if err != nil {
		return err
	}
	if u.isSameCustomAPIBase(parsed) {
		return nil
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("release asset redirect for %s must use https: %s", assetName, rawURL)
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com", "github-releases.githubusercontent.com":
		return nil
	default:
		return fmt.Errorf("release asset redirect for %s uses unexpected host %q: %s", assetName, parsed.Hostname(), rawURL)
	}
}

func parseDownloadURL(rawURL, assetName, context string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid %s for %s: %w", context, assetName, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid %s for %s: URL must be absolute: %s", context, assetName, rawURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("invalid %s for %s: URL must not contain user information: %s", context, assetName, rawURL)
	}
	return parsed, nil
}

func (u *Updater) isSameCustomAPIBase(parsed *url.URL) bool {
	apiBase, err := url.Parse(u.APIBase)
	if err != nil || apiBase.Scheme == "" || apiBase.Host == "" {
		return false
	}
	if strings.EqualFold(apiBase.Hostname(), "api.github.com") {
		return false
	}
	return strings.EqualFold(parsed.Scheme, apiBase.Scheme) && strings.EqualFold(parsed.Host, apiBase.Host)
}

func requestTimeout(configured, fallback time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return fallback
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return true
		}
	}
	type timeout interface {
		Timeout() bool
	}
	var timeoutErr timeout
	return errors.As(err, &timeoutErr) && timeoutErr.Timeout()
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
