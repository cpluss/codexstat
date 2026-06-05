package main

import (
	"os"
	"path/filepath"
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
