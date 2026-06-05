package codex

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	reset := now.Add(2 * time.Hour)
	snapshot := &Snapshot{
		Provider:  "codex",
		Source:    SourceOAuth,
		UpdatedAt: now,
		Session:   &Window{UsedPercent: 25, RemainingPercent: 75, ResetsAt: &reset, ResetIn: "old"},
		TokenUsage: &TokenUsageReport{
			Metric: "tokens",
			Total:  TokenUsageTotal{Total: 100},
		},
	}

	if err := RecordSnapshotCache(path, snapshot, now); err != nil {
		t.Fatal(err)
	}
	cached, fresh, err := FreshSnapshotFromCache(path, now.Add(30*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		t.Fatal("snapshot cache should be fresh")
	}
	if cached.TokenUsage != nil {
		t.Fatalf("snapshot cache should not persist token usage: %#v", cached.TokenUsage)
	}
	if cached.Session == nil || cached.Session.ResetIn != "in 1h 59m" {
		t.Fatalf("reset countdown was not refreshed: %#v", cached.Session)
	}
}

func TestSnapshotCacheReportsStaleSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	snapshot := &Snapshot{
		Provider:  "codex",
		Source:    SourceOAuth,
		UpdatedAt: now,
		Session:   &Window{UsedPercent: 25, RemainingPercent: 75},
	}
	if err := RecordSnapshotCache(path, snapshot, now); err != nil {
		t.Fatal(err)
	}

	cached, fresh, err := FreshSnapshotFromCache(path, now.Add(10*time.Minute), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if cached == nil {
		t.Fatal("stale snapshot should still be returned for fallback")
	}
	if fresh {
		t.Fatal("snapshot cache should be stale")
	}
}

func TestDefaultSnapshotCachePathUsesOverride(t *testing.T) {
	path, err := DefaultSnapshotCachePath(map[string]string{
		"CODEXSTAT_SNAPSHOT_CACHE": "~/custom/snapshot.json",
		"HOME":                     "/tmp/home",
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/home/custom/snapshot.json" {
		t.Fatalf("got %q", path)
	}
}
