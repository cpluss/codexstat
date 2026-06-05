package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildTokenUsageReportScansCodexSessions(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "30")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "rollout-2026-05-30T10-00-00-test.jsonl")
	contents := strings.Join([]string{
		`{"timestamp":"2026-05-30T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":null}}`,
		`{"timestamp":"2026-05-30T09:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":1050},"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":1050}}}}`,
		`{"timestamp":"2026-05-30T09:02:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1400,"cached_input_tokens":300,"output_tokens":90,"reasoning_output_tokens":20,"total_tokens":1490},"last_token_usage":{"input_tokens":400,"cached_input_tokens":100,"output_tokens":40,"reasoning_output_tokens":10,"total_tokens":440}}}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := BuildTokenUsageReport(TokenUsageQuery{
		Days:      2,
		Metric:    "tokens",
		Now:       time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
		CodexHome: home,
		Env:       map[string]string{"HOME": home},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesScanned != 1 || report.EventsScanned != 2 {
		t.Fatalf("unexpected scan counts: files=%d events=%d", report.FilesScanned, report.EventsScanned)
	}
	if len(report.Days) != 2 {
		t.Fatalf("got %d days, want 2", len(report.Days))
	}
	day := report.Days[0]
	if day.Date != "2026-05-30" {
		t.Fatalf("unexpected day: %#v", day)
	}
	if day.Sessions != 1 || day.Events != 2 {
		t.Fatalf("unexpected day counts: %#v", day)
	}
	if day.Tokens.Input != 1400 || day.Tokens.Cached != 300 || day.Tokens.Output != 90 ||
		day.Tokens.Reasoning != 20 || day.Tokens.Total != 1490 {
		t.Fatalf("unexpected token totals: %#v", day.Tokens)
	}
	if report.Total.Total != 1490 {
		t.Fatalf("unexpected report total: %#v", report.Total)
	}
}

func TestBuildTokenUsageReportUsesCacheForUnchangedFiles(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "31")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "rollout-2026-05-31T10-00-00-test.jsonl")
	cachePath := filepath.Join(home, "cache", "token_usage.sqlite")
	modTime := time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)

	validLine := fmt.Sprintf(
		`{"timestamp":"2026-05-31T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110}}},"padding":%q}`,
		strings.Repeat("x", 120),
	)
	validContents := validLine + "\n"
	if err := os.WriteFile(path, []byte(validContents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}

	query := TokenUsageQuery{
		Metric:    "tokens",
		Now:       time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
		CodexHome: home,
		CachePath: cachePath,
		Env:       map[string]string{"HOME": home},
	}
	report, err := BuildTokenUsageReport(query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Total != 110 {
		t.Fatalf("unexpected initial total: %#v", report.Total)
	}

	invalidLine := `{"timestamp":"not-a-time","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":999,"output_tokens":1,"total_tokens":1000}}}}`
	if len(invalidLine)+1 > len(validContents) {
		t.Fatalf("invalid fixture is longer than valid fixture")
	}
	invalidContents := invalidLine + strings.Repeat(" ", len(validContents)-len(invalidLine)-1) + "\n"
	if len(invalidContents) != len(validContents) {
		t.Fatalf("fixture length mismatch: got %d, want %d", len(invalidContents), len(validContents))
	}
	if err := os.WriteFile(path, []byte(invalidContents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}

	report, err = BuildTokenUsageReport(query)
	if err != nil {
		t.Fatalf("unchanged file should be served from cache: %v", err)
	}
	if report.Total.Total != 110 || report.EventsScanned != 1 {
		t.Fatalf("unexpected cached report: %#v", report)
	}
}

func TestBuildTokenUsageReportRefreshesChangedCacheFiles(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "31")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "rollout-2026-05-31T10-00-00-test.jsonl")
	cachePath := filepath.Join(home, "cache", "token_usage.sqlite")

	firstLine := `{"timestamp":"2026-05-31T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110}}}}`
	if err := os.WriteFile(path, []byte(firstLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	query := TokenUsageQuery{
		Metric:    "tokens",
		Now:       time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
		CodexHome: home,
		CachePath: cachePath,
		Env:       map[string]string{"HOME": home},
	}
	report, err := BuildTokenUsageReport(query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Total != 110 || report.EventsScanned != 1 {
		t.Fatalf("unexpected initial report: %#v", report)
	}

	secondLine := `{"timestamp":"2026-05-31T09:05:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":50,"output_tokens":20,"total_tokens":70}}}}`
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(secondLine + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	report, err = BuildTokenUsageReport(query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Total != 180 || report.EventsScanned != 2 {
		t.Fatalf("changed file was not refreshed: %#v", report)
	}
}

func TestBuildTokenUsageReportWarmCacheSkipsOlderFiles(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "token_usage.sqlite")
	oldPath := filepath.Join(home, "sessions", "2026", "05", "01", "rollout-2026-05-01T10-00-00-old.jsonl")
	todayPath := filepath.Join(home, "sessions", "2026", "05", "31", "rollout-2026-05-31T10-00-00-today.jsonl")

	for _, path := range []string{oldPath, todayPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	oldLine := `{"timestamp":"2026-05-01T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110}}}}`
	todayLine := `{"timestamp":"2026-05-31T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"output_tokens":20,"total_tokens":220}}}}`
	if err := os.WriteFile(oldPath, []byte(oldLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(todayPath, []byte(todayLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	query := TokenUsageQuery{
		Metric:    "tokens",
		Now:       time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
		CodexHome: home,
		CachePath: cachePath,
		Env:       map[string]string{"HOME": home},
	}
	report, err := BuildTokenUsageReport(query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Total != 330 || report.EventsScanned != 2 {
		t.Fatalf("unexpected initial report: %#v", report)
	}

	invalidOldLine := `{"timestamp":"not-a-time","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":999,"output_tokens":1,"total_tokens":1000}}}}`
	if err := os.WriteFile(oldPath, []byte(invalidOldLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err = BuildTokenUsageReport(query)
	if err != nil {
		t.Fatalf("warm cache should not reparse older files: %v", err)
	}
	if report.Total.Total != 330 || report.EventsScanned != 2 {
		t.Fatalf("older cached aggregate changed unexpectedly: %#v", report)
	}
}

func TestBuildTokenUsageReportWarmCacheRefreshesPreviousDay(t *testing.T) {
	home := t.TempDir()
	cachePath := filepath.Join(home, "cache", "token_usage.sqlite")
	yesterdayPath := filepath.Join(home, "sessions", "2026", "05", "30", "rollout-2026-05-30T23-55-00-yesterday.jsonl")
	todayPath := filepath.Join(home, "sessions", "2026", "05", "31", "rollout-2026-05-31T10-00-00-today.jsonl")

	for _, path := range []string{yesterdayPath, todayPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	yesterdayLine := `{"timestamp":"2026-05-30T22:55:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110}}}}`
	todayLine := `{"timestamp":"2026-05-31T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"output_tokens":20,"total_tokens":220}}}}`
	if err := os.WriteFile(yesterdayPath, []byte(yesterdayLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(todayPath, []byte(todayLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	query := TokenUsageQuery{
		Metric:    "tokens",
		Now:       time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
		CodexHome: home,
		CachePath: cachePath,
		Env:       map[string]string{"HOME": home},
	}
	report, err := BuildTokenUsageReport(query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Total != 330 || report.EventsScanned != 2 {
		t.Fatalf("unexpected initial report: %#v", report)
	}

	appendLine := `{"timestamp":"2026-05-31T00:05:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":50,"output_tokens":5,"total_tokens":55}}}}`
	file, err := os.OpenFile(yesterdayPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(appendLine + "\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	report, err = BuildTokenUsageReport(query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total.Total != 385 || report.EventsScanned != 3 {
		t.Fatalf("previous-day session was not refreshed: %#v", report)
	}
}

func TestBuildTokenUsageReportUsesOutputMetric(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)
	home := t.TempDir()
	sessionDir := filepath.Join(home, "sessions", "2026", "05", "31")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessionDir, "rollout-2026-05-31T10-00-00-test.jsonl")
	line := `{"timestamp":"2026-05-31T09:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125}}}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := BuildTokenUsageReport(TokenUsageQuery{
		Days:      1,
		Metric:    "output",
		Now:       now,
		CodexHome: home,
		Env:       map[string]string{"HOME": home},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 1 {
		t.Fatalf("got %d days, want 1", len(report.Days))
	}
	if report.Days[0].Graph != 25 {
		t.Fatalf("got graph value %d, want 25", report.Days[0].Graph)
	}
}

func TestBuildTokenUsageReportDefaultsToAvailableHistory(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)
	home := t.TempDir()

	writeTokenFile := func(path, timestamp string, total int64) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		line := fmt.Sprintf(
			`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":%d,"output_tokens":10,"total_tokens":%d}}}}`,
			timestamp,
			total,
			total+10,
		)
		if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeTokenFile(
		filepath.Join(home, "sessions", "2026", "05", "20", "rollout-2026-05-20T10-00-00-old.jsonl"),
		"2026-05-20T09:00:00Z",
		100,
	)
	writeTokenFile(
		filepath.Join(home, "sessions", "2026", "05", "22", "rollout-2026-05-22T10-00-00-new.jsonl"),
		"2026-05-22T09:00:00Z",
		300,
	)

	report, err := BuildTokenUsageReport(TokenUsageQuery{
		Metric:    "tokens",
		Now:       now,
		CodexHome: home,
		Env:       map[string]string{"HOME": home},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Since != "2026-05-20" || report.Until != "2026-05-22" {
		t.Fatalf("unexpected report bounds: %#v", report)
	}
	if len(report.Days) != 3 {
		t.Fatalf("got %d days, want 3", len(report.Days))
	}
	if report.Days[1].Date != "2026-05-21" || report.Days[1].Tokens.Total != 0 {
		t.Fatalf("missing zero-filled gap day: %#v", report.Days[1])
	}
	if report.FilesScanned != 2 || report.EventsScanned != 2 {
		t.Fatalf("unexpected scan counts: files=%d events=%d", report.FilesScanned, report.EventsScanned)
	}
	if report.Total.Total != 420 {
		t.Fatalf("unexpected total: %#v", report.Total)
	}
}

func TestRenderTokenUsageShowsDayOverDayGraph(t *testing.T) {
	report := TokenUsageReport{
		Metric:        "tokens",
		Since:         "2026-05-30",
		Until:         "2026-05-31",
		FilesScanned:  1,
		EventsScanned: 1,
		Total:         TokenUsageTotal{Total: 3000},
		Days: []TokenUsageDay{
			{Date: "2026-05-30", Tokens: TokenUsageTotal{Total: 1000}, Graph: 1000},
			{Date: "2026-05-31", Tokens: TokenUsageTotal{Total: 2000}, Graph: 2000},
		},
	}
	out := RenderTokenUsage(report, RenderOptions{
		Now: time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
	})
	if !strings.Contains(out, "Codex token usage") {
		t.Fatalf("missing title:\n%s", out)
	}
	if !strings.Contains(out, "Summary") {
		t.Fatalf("missing token usage summary:\n%s", out)
	}
	if !strings.Contains(out, "Monthly usage") {
		t.Fatalf("missing monthly usage section:\n%s", out)
	}
	if !strings.Contains(out, "Daily usage, last 30 days") {
		t.Fatalf("missing time chart:\n%s", out)
	}
	if !strings.Contains(out, "Recent days") {
		t.Fatalf("missing recent days table:\n%s", out)
	}
	if strings.Contains(out, "#") {
		t.Fatalf("output should not use ASCII hash graphs:\n%s", out)
	}
	if strings.Contains(out, "Graph") {
		t.Fatalf("day summary should not include a graph column:\n%s", out)
	}
	for _, wanted := range []string{"Today", "Last 7 days", "This month", "All local", "2026-05", "2/31", "2026-05-31 2K"} {
		if !strings.Contains(out, wanted) {
			t.Fatalf("missing token usage detail %q:\n%s", wanted, out)
		}
	}
	for _, unwanted := range []string{"Aggregate", "files", "events", "Sessions", "Events", "roots", "Cached", "Reason"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("pretty output includes unwanted detail %q:\n%s", unwanted, out)
		}
	}
}

func TestRenderTokenUsageShowsMonthlyActiveDaysAndPeaks(t *testing.T) {
	report := TokenUsageReport{
		Metric: "tokens",
		Since:  "2026-04-08",
		Until:  "2026-06-05",
		Total:  TokenUsageTotal{Total: 762_000_000},
		Days: []TokenUsageDay{
			{Date: "2026-04-08", Tokens: TokenUsageTotal{Total: 508_000_000}},
			{Date: "2026-04-09", Tokens: TokenUsageTotal{Total: 100_000_000}},
			{Date: "2026-05-31", Tokens: TokenUsageTotal{Total: 124_000_000}},
			{Date: "2026-06-01", Tokens: TokenUsageTotal{Total: 10_000_000}},
			{Date: "2026-06-05", Tokens: TokenUsageTotal{Total: 20_000_000}},
		},
	}

	out := RenderTokenUsage(report, RenderOptions{
		Now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.Local),
	})

	for _, wanted := range []string{
		"Today",
		"Last 7 days",
		"This month",
		"Last month",
		"All local",
		"154M",
		"762M",
		"2026-04",
		"2/30",
		"608M",
		"2026-04-08 508M",
		"2026-05",
		"1/31",
		"2026-05-31 124M",
		"2026-06",
		"2/5",
		"30M",
		"2026-06-05 20M",
	} {
		if !strings.Contains(out, wanted) {
			t.Fatalf("missing token usage detail %q:\n%s", wanted, out)
		}
	}
}

func TestEmptyTokenUsageReportIncludesDayBuckets(t *testing.T) {
	report := EmptyTokenUsageReport(TokenUsageQuery{
		Days:   3,
		Metric: "tokens",
		Now:    time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
	})
	if report.Metric != "tokens" || report.Since != "2026-05-29" || report.Until != "2026-05-31" {
		t.Fatalf("unexpected report bounds: %#v", report)
	}
	if len(report.Days) != 3 {
		t.Fatalf("got %d days, want 3", len(report.Days))
	}
}
