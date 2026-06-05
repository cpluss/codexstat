package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultUpdateRepo = "cpluss/codexstat"
	updateBinaryName  = "codexstat"
)

type updateOptions struct {
	Version string
	Stdout  io.Writer
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	APIURL             string `json:"url"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func selfUpdate(ctx context.Context, opts updateOptions) error {
	repo := strings.TrimSpace(os.Getenv("CODEXSTAT_REPO"))
	if repo == "" {
		repo = defaultUpdateRepo
	}
	token := githubToken()

	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = strings.TrimSpace(os.Getenv("CODEXSTAT_VERSION"))
	}
	if version == "" {
		version = "latest"
	}

	installPath, err := updateInstallPath()
	if err != nil {
		return err
	}

	target, err := updateTarget()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	release, err := fetchRelease(ctx, client, repo, version, token)
	if err != nil {
		return err
	}

	assetName := updateAssetName(target)
	asset, ok := releaseAsset(release, assetName)
	if !ok {
		return fmt.Errorf("release %s has no asset named %s", release.TagName, assetName)
	}
	checksums, ok := releaseAsset(release, "checksums.txt")
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt asset", release.TagName)
	}

	tmpDir, err := os.MkdirTemp("", "codexstat-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	assetPath := filepath.Join(tmpDir, assetName)
	if err := downloadReleaseAsset(ctx, client, asset, assetPath, token); err != nil {
		return err
	}

	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadReleaseAsset(ctx, client, checksums, checksumsPath, token); err != nil {
		return err
	}
	if err := verifyDownloadedChecksum(assetPath, checksumsPath, assetName); err != nil {
		return err
	}

	extractedPath, err := extractUpdateBinary(assetPath, tmpDir)
	if err != nil {
		return err
	}

	if err := installUpdateBinary(extractedPath, installPath); err != nil {
		return err
	}

	out := opts.Stdout
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "codexstat: updated to %s at %s\n", release.TagName, installPath)
	return nil
}

func githubToken() string {
	for _, key := range []string{"CODEXSTAT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(key)); token != "" {
			return token
		}
	}
	return ""
}

func updateInstallPath() (string, error) {
	if installDir := strings.TrimSpace(os.Getenv("CODEXSTAT_INSTALL_DIR")); installDir != "" {
		return filepath.Join(expandHome(installDir), updateBinaryName), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return exe, nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func updateTarget() (string, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
	default:
		return "", fmt.Errorf("self-update is not supported on %s", runtime.GOOS)
	}

	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("self-update is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return runtime.GOOS + "_" + runtime.GOARCH, nil
}

func updateAssetName(target string) string {
	return updateBinaryName + "_" + target + ".tar.gz"
}

func fetchRelease(ctx context.Context, client *http.Client, repo, version, token string) (githubRelease, error) {
	endpoint := "latest"
	if version != "latest" {
		if version == "main" {
			version = "nightly"
		}
		endpoint = "tags/" + url.PathEscape(version)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/"+endpoint, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "codexstat/"+versionString())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("GitHub release lookup failed: %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	if release.TagName == "" {
		return githubRelease{}, errors.New("GitHub release response did not include a tag name")
	}
	return release, nil
}

func releaseAsset(release githubRelease, name string) (githubReleaseAsset, bool) {
	for _, asset := range release.Assets {
		if asset.Name == name && (asset.BrowserDownloadURL != "" || asset.APIURL != "") {
			return asset, true
		}
	}
	return githubReleaseAsset{}, false
}

func downloadReleaseAsset(ctx context.Context, client *http.Client, asset githubReleaseAsset, destinationPath, token string) error {
	sourceURL := asset.BrowserDownloadURL
	accept := ""
	if token != "" && asset.APIURL != "" {
		sourceURL = asset.APIURL
		accept = "application/octet-stream"
	}
	if sourceURL == "" {
		return fmt.Errorf("release asset %s has no download URL", asset.Name)
	}
	return downloadFile(ctx, client, sourceURL, destinationPath, token, accept)
}

func downloadFile(ctx context.Context, client *http.Client, sourceURL, destinationPath, token, accept string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "codexstat/"+versionString())
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed for %s: %s", sourceURL, resp.Status)
	}

	out, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func verifyDownloadedChecksum(assetPath, checksumsPath, assetName string) error {
	expected, err := checksumForAsset(checksumsPath, assetName)
	if err != nil {
		return err
	}

	file, err := os.Open(assetPath)
	if err != nil {
		return err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	if expected != actual {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func checksumForAsset(checksumsPath, assetName string) (string, error) {
	content, err := os.ReadFile(checksumsPath)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			sum := strings.ToLower(fields[0])
			if len(sum) != sha256.Size*2 {
				return "", fmt.Errorf("invalid checksum for %s", assetName)
			}
			if _, err := hex.DecodeString(sum); err != nil {
				return "", fmt.Errorf("invalid checksum for %s", assetName)
			}
			return sum, nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found in checksums.txt", assetName)
}

func extractUpdateBinary(archivePath, tmpDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != updateBinaryName {
			continue
		}

		path := filepath.Join(tmpDir, updateBinaryName)
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(out, tarReader)
		closeErr := out.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if err := os.Chmod(path, 0755); err != nil {
			return "", err
		}
		return path, nil
	}

	return "", fmt.Errorf("%s did not contain %s", filepath.Base(archivePath), updateBinaryName)
}

func installUpdateBinary(sourcePath, installPath string) error {
	installDir := filepath.Dir(installPath)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	tmpPath := filepath.Join(installDir, "."+filepath.Base(installPath)+".tmp")
	target, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, installPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
