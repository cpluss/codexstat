package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecksumForAsset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checksums.txt")
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  codexstat_darwin_arm64.tar.gz\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	sum, err := checksumForAsset(path, "codexstat_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if sum != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("checksumForAsset returned %q", sum)
	}
}

func TestChecksumForAssetRejectsMissingAsset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checksums.txt")
	content := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  codexstat_linux_arm64.tar.gz\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := checksumForAsset(path, "codexstat_darwin_arm64.tar.gz"); err == nil {
		t.Fatal("checksumForAsset succeeded for missing asset")
	}
}

func TestReleaseAssetSelectsNamedAsset(t *testing.T) {
	release := githubRelease{
		Assets: []githubReleaseAsset{
			{Name: "checksums.txt", APIURL: "https://api.example.test/checksums"},
			{Name: "codexstat_darwin_arm64.tar.gz", APIURL: "https://api.example.test/asset"},
		},
	}

	asset, ok := releaseAsset(release, "codexstat_darwin_arm64.tar.gz")
	if !ok {
		t.Fatal("releaseAsset did not find named asset")
	}
	if asset.APIURL != "https://api.example.test/asset" {
		t.Fatalf("APIURL = %q", asset.APIURL)
	}
}

func TestNewerReleaseAvailableComparesReleaseVersions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "newer patch", current: "v0.4.4", latest: "v0.4.5", want: true},
		{name: "newer minor", current: "v0.4.9", latest: "v0.5.0", want: true},
		{name: "same version", current: "v0.4.4", latest: "v0.4.4", want: false},
		{name: "latest older", current: "v0.4.5", latest: "v0.4.4", want: false},
		{name: "current without v", current: "0.4.4", latest: "v0.4.5", want: true},
		{name: "current from git describe", current: "v0.4.4-1-gabcdef", latest: "v0.4.5", want: true},
		{name: "dev build", current: "dev (abcdef123456)", latest: "v0.4.5", want: false},
		{name: "nightly build", current: "nightly", latest: "v0.4.5", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := newerReleaseAvailable(tt.current, tt.latest)
			if got != tt.want {
				t.Fatalf("newerReleaseAvailable(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestUpdateNoticeForReleaseUsesLatestTag(t *testing.T) {
	notice, ok := updateNoticeForRelease("v0.4.4", githubRelease{TagName: "v0.4.5"})
	if !ok {
		t.Fatal("updateNoticeForRelease did not report newer release")
	}
	if notice.CurrentVersion != "v0.4.4" || notice.LatestVersion != "v0.4.5" {
		t.Fatalf("notice = %+v", notice)
	}

	if _, ok := updateNoticeForRelease("v0.4.5", githubRelease{TagName: "v0.4.5"}); ok {
		t.Fatal("updateNoticeForRelease reported current release as newer")
	}
}

func TestRenderUpdateNoticeIncludesUpdateCommand(t *testing.T) {
	out := renderUpdateNotice(updateNotice{
		CurrentVersion: "v0.4.4",
		LatestVersion:  "v0.4.5",
	}, false)

	for _, wanted := range []string{
		"New codexstat release available: v0.4.5 (current v0.4.4)",
		"Update with: codexstat update",
	} {
		if !strings.Contains(out, wanted) {
			t.Fatalf("renderUpdateNotice missing %q:\n%s", wanted, out)
		}
	}
	for _, unwanted := range []string{"+", "|"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("renderUpdateNotice should not render a border:\n%s", out)
		}
	}
}

func TestRenderUpdateNoticeUsesYellowWhenColorEnabled(t *testing.T) {
	out := renderUpdateNotice(updateNotice{
		CurrentVersion: "v0.4.4",
		LatestVersion:  "v0.4.5",
	}, true)

	if !strings.HasPrefix(out, "\x1b[33m") || !strings.HasSuffix(out, "\x1b[0m") {
		t.Fatalf("renderUpdateNotice should wrap colored output in yellow ANSI codes: %q", out)
	}
}
