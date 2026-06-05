package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const snapshotCacheVersion = 1

type SnapshotCacheRecord struct {
	Version  int       `json:"version"`
	CachedAt time.Time `json:"cached_at"`
	Snapshot Snapshot  `json:"snapshot"`
}

func DefaultSnapshotCachePath(env map[string]string) (string, error) {
	if env == nil {
		env = environMap()
	}
	if path := strings.TrimSpace(env["CODEXSTAT_SNAPSHOT_CACHE"]); path != "" {
		return expandHome(path, env), nil
	}
	if base := strings.TrimSpace(env["XDG_CACHE_HOME"]); base != "" {
		return filepath.Join(expandHome(base, env), "codexstat", "snapshot.json"), nil
	}
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", "codexstat", "snapshot.json"), nil
	}
	return filepath.Join(home, ".cache", "codexstat", "snapshot.json"), nil
}

func RecordSnapshotCache(path string, snapshot *Snapshot, now time.Time) error {
	if path == "" {
		return errors.New("missing snapshot cache path")
	}
	if snapshot == nil {
		return errors.New("nil snapshot")
	}
	record := SnapshotCacheRecord{
		Version:  snapshotCacheVersion,
		CachedAt: now,
		Snapshot: *snapshot,
	}
	record.Snapshot.TokenUsage = nil

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapshot.json.")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func LoadSnapshotCache(path string, now time.Time) (*SnapshotCacheRecord, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record SnapshotCacheRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record.Version != snapshotCacheVersion {
		return nil, fmt.Errorf("unsupported snapshot cache version %d", record.Version)
	}
	if record.Snapshot.Provider == "" {
		return nil, errors.New("snapshot cache missing snapshot")
	}
	refreshSnapshotResetCountdowns(&record.Snapshot, now)
	return &record, nil
}

func FreshSnapshotFromCache(path string, now time.Time, maxAge time.Duration) (*Snapshot, bool, error) {
	record, err := LoadSnapshotCache(path, now)
	if err != nil || record == nil {
		return nil, false, err
	}
	if maxAge < 0 {
		maxAge = 0
	}
	if record.CachedAt.IsZero() || now.Sub(record.CachedAt) > maxAge {
		return &record.Snapshot, false, nil
	}
	return &record.Snapshot, true, nil
}

func refreshSnapshotResetCountdowns(snapshot *Snapshot, now time.Time) {
	if snapshot == nil {
		return
	}
	refreshWindowResetCountdown(snapshot.Session, now)
	refreshWindowResetCountdown(snapshot.Weekly, now)
	for i := range snapshot.Extra {
		refreshWindowResetCountdown(&snapshot.Extra[i].Window, now)
	}
}

func refreshWindowResetCountdown(window *Window, now time.Time) {
	if window == nil || window.ResetsAt == nil {
		return
	}
	window.ResetIn = resetCountdown(*window.ResetsAt, now)
}
