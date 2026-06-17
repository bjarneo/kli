// Package upgrade implements self-upgrade by downloading the latest release
// binary from GitHub.
package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "bjarneo/ku"

var httpClient = &http.Client{Timeout: 30 * time.Second}

type release struct {
	TagName string `json:"tag_name"`
}

// Run checks for a newer GitHub release and replaces the current binary.
func Run(currentVersion string) error {
	latest, err := latestVersion()
	if err != nil {
		return fmt.Errorf("checking latest version: %w", err)
	}

	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion != "" && currentVersion != "dev" && currentVersion == latest {
		fmt.Printf("Already up to date (%s)\n", currentVersion)
		return nil
	}

	if currentVersion == "" || currentVersion == "dev" {
		fmt.Printf("Latest release is %s, downloading...\n", latest)
	} else {
		fmt.Printf("Upgrading %s -> %s\n", currentVersion, latest)
	}

	asset, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, asset)
	checksumURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/checksums.txt", repo)

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}

	checksum, err := latestChecksum(checksumURL, asset)
	if err != nil {
		return fmt.Errorf("checking checksum: %w", err)
	}
	if err := downloadAndReplace(url, exe, checksum); err != nil {
		return err
	}

	fmt.Printf("Upgraded to %s\n", latest)
	return nil
}

// Latest returns the most recent published release tag (e.g. "v0.3.0"). It is
// the read-only half of Run, used by the UI to check for updates in the
// background without touching the binary.
func Latest() (string, error) {
	return latestVersion()
}

// IsNewer reports whether latest is a strictly greater release than current.
// A development or unset current ("", "dev") returns false: there is no
// released version to compare against, so there is nothing to nag about.
func IsNewer(current, latest string) bool {
	current = strings.TrimSpace(current)
	if current == "" || current == "dev" {
		return false
	}
	return compareSemver(latest, current) > 0
}

// compareSemver returns 1 if a > b, -1 if a < b, and 0 if equal, comparing the
// major.minor.patch triplets and ignoring any pre-release or build suffix.
func compareSemver(a, b string) int {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		switch {
		case pa[i] > pb[i]:
			return 1
		case pa[i] < pb[i]:
			return -1
		}
	}
	return 0
}

// parseSemver extracts the leading major.minor.patch numbers from a tag like
// "v1.2.3" or "1.2.3-rc1". Missing or non-numeric parts read as 0.
func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var v [3]int
	for i, part := range strings.SplitN(s, ".", 3) {
		v[i], _ = strconv.Atoi(strings.TrimSpace(part))
	}
	return v
}

func assetName(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	name := fmt.Sprintf("ku-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}

func latestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var r release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if r.TagName == "" {
		return "", fmt.Errorf("release has no tag_name")
	}
	return r.TagName, nil
}

func latestChecksum(url, asset string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", asset)
}

func downloadAndReplace(url, destPath, checksum string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, "ku-upgrade-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w (try running with sudo)", err)
	}
	tmpPath := tmp.Name()

	const maxBinarySize = 200 << 20
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, maxBinarySize)); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing binary: %w", err)
	}

	if err := verifyFileChecksum(tmpPath, checksum); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replacing binary: %w (try running with sudo)", err)
	}
	return nil
}
