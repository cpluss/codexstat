package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	now := time.Date(2026, 5, 31, 9, 0, 0, 0, time.Local)

	snapshot := &Snapshot{
		Provider:  "codex",
		Source:    SourceOAuth,
		UpdatedAt: now,
		Session:   &Window{UsedPercent: 20, RemainingPercent: 80},
		Weekly:    &Window{UsedPercent: 15, RemainingPercent: 85},
		Credits:   &Credits{Balance: ptr(1.0)},
	}

	if err := RecordSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}

	records, err := LoadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Snapshot.Weekly == nil || records[0].Snapshot.Weekly.UsedPercent != 15 {
		t.Fatalf("unexpected record: %#v", records[0])
	}
}

func TestBuildHistoryReportBucketsExplicitRecords(t *testing.T) {
	now := time.Date(2026, 5, 31, 9, 0, 0, 0, time.Local)
	records := []HistoryRecord{
		{
			Version:    historyVersion,
			CapturedAt: now.AddDate(0, 0, -1),
			Snapshot: Snapshot{
				Provider:  "codex",
				Source:    SourceOAuth,
				UpdatedAt: now.AddDate(0, 0, -1),
				Session:   &Window{UsedPercent: 40, RemainingPercent: 60},
				Weekly:    &Window{UsedPercent: 10, RemainingPercent: 90},
				Credits:   &Credits{Balance: ptr(1.5)},
			},
		},
		{
			Version:    historyVersion,
			CapturedAt: now,
			Snapshot: Snapshot{
				Provider:  "codex",
				Source:    SourceOAuth,
				UpdatedAt: now,
				Session:   &Window{UsedPercent: 20, RemainingPercent: 80},
				Weekly:    &Window{UsedPercent: 15, RemainingPercent: 85},
				Credits:   &Credits{Balance: ptr(1.0)},
			},
		},
	}

	report, err := BuildHistoryReport(records, HistoryQuery{
		Days:   2,
		Metric: "weekly",
		Now:    now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 2 {
		t.Fatalf("got %d days, want 2", len(report.Days))
	}
	if report.MatchedRows != 2 {
		t.Fatalf("got %d matched rows, want 2", report.MatchedRows)
	}
	if report.Days[1].WeeklyLastUsed == nil || *report.Days[1].WeeklyLastUsed != 15 {
		t.Fatalf("unexpected latest day: %#v", report.Days[1])
	}
}

func TestDefaultHistoryPathUsesOverride(t *testing.T) {
	path, err := DefaultHistoryPath(map[string]string{
		"CODEXSTAT_HISTORY": "~/custom/history.jsonl",
		"HOME":              "/tmp/home",
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/home/custom/history.jsonl" {
		t.Fatalf("got %q", path)
	}
}

func TestRenderHistoryShowsGraph(t *testing.T) {
	value := 25.0
	report := HistoryReport{
		Metric:      "weekly",
		Since:       "2026-05-31",
		Until:       "2026-05-31",
		TotalRows:   1,
		MatchedRows: 1,
		Days: []HistoryDay{{
			Date:           "2026-05-31",
			Samples:        1,
			WeeklyLastUsed: &value,
			GraphValue:     &value,
		}},
	}
	out := RenderHistory(report, RenderOptions{})
	if !strings.Contains(out, "[#####---------------] 25%") {
		t.Fatalf("missing graph in output:\n%s", out)
	}
}

func TestLoadHistoryReturnsEmptyForMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.jsonl")
	records, err := LoadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Fatalf("got %#v, want nil", records)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("history file should not be created by load")
	}
}
