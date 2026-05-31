package codex

import (
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

func TestRenderTokenUsageShowsDayOverDayGraph(t *testing.T) {
	report := TokenUsageReport{
		Metric:        "tokens",
		Since:         "2026-05-30",
		Until:         "2026-05-31",
		FilesScanned:  1,
		EventsScanned: 1,
		Total:         TokenUsageTotal{Total: 2000},
		Days: []TokenUsageDay{
			{Date: "2026-05-30", Tokens: TokenUsageTotal{Total: 1000}, Graph: 1000},
			{Date: "2026-05-31", Tokens: TokenUsageTotal{Total: 2000}, Graph: 2000},
		},
	}
	out := RenderTokenUsage(report, RenderOptions{})
	if !strings.Contains(out, "Codex token usage") {
		t.Fatalf("missing title:\n%s", out)
	}
	if !strings.Contains(out, "Tokens/day graph") {
		t.Fatalf("missing time chart:\n%s", out)
	}
	if strings.Contains(out, "#") {
		t.Fatalf("output should not use ASCII hash graphs:\n%s", out)
	}
	if strings.Contains(out, "Graph") {
		t.Fatalf("day summary should not include a graph column:\n%s", out)
	}
	if !strings.Contains(out, "Aggregate") {
		t.Fatalf("missing aggregate row:\n%s", out)
	}
	if !strings.Contains(out, "│ Aggregate") {
		t.Fatalf("aggregate should remain in the daily table:\n%s", out)
	}
	if !strings.Contains(out, "│ "+tableSeparatorCell) {
		t.Fatalf("missing separator before aggregate row:\n%s", out)
	}
	if strings.Contains(out, "aggregate ") {
		t.Fatalf("chart footer should not repeat aggregate:\n%s", out)
	}
	for _, unwanted := range []string{"files", "events", "Sessions", "Events", "roots"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("pretty output includes unwanted detail %q:\n%s", unwanted, out)
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
