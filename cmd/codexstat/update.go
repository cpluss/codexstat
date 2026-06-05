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
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func selfUpdate(ctx context.Context, opts updateOptions) error {
	repo := strings.TrimSpace(os.Getenv("CODEXSTAT_REPO"))
	if repo == "" {
		repo = defaultUpdateRepo
	}

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
	release, err := fetchRelease(ctx, client, repo, version)
	if err != nil {
		return err
	}

	assetName := updateAssetName(target)
	assetURL, ok := releaseAssetURL(release, assetName)
	if !ok {
		return fmt.Errorf("release %s has no asset named %s", release.TagName, assetName)
	}
	checksumsURL, ok := releaseAssetURL(release, "checksums.txt")
	if !ok {
		return fmt.Errorf("release %s has no checksums.txt asset", release.TagName)
	}

	tmpDir, err := os.MkdirTemp("", "codexstat-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	assetPath := filepath.Join(tmpDir, assetName)
	if err := downloadFile(ctx, client, assetURL, assetPath); err != nil {
		return err
	}

	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(ctx, client, checksumsURL, checksumsPath); err != nil {
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

func fetchRelease(ctx context.Context, client *http.Client, repo, version string) (githubRelease, error) {
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

func releaseAssetURL(release githubRelease, name string) (string, bool) {
	for _, asset := range release.Assets {
		if asset.Name == name && asset.BrowserDownloadURL != "" {
			return asset.BrowserDownloadURL, true
		}
	}
	return "", false
}

func downloadFile(ctx context.Context, client *http.Client, sourceURL, destinationPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "codexstat/"+versionString())

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
