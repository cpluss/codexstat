package codex

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTextIncludesTokenUsage(t *testing.T) {
	snapshot := &Snapshot{
		Provider:  "codex",
		Source:    SourceOAuth,
		UpdatedAt: time.Date(2026, 5, 31, 9, 30, 0, 0, time.Local),
		Account:   &Account{Email: "me@example.com", Plan: "pro", AccountID: "acct_123"},
		Credits:   &Credits{HasCredits: true, Balance: ptr(12.5)},
		TokenUsage: &TokenUsageReport{
			Metric:        "tokens",
			Since:         "2026-05-25",
			Until:         "2026-05-31",
			FilesScanned:  1,
			EventsScanned: 1,
			Total:         TokenUsageTotal{Total: 1200},
			Days: []TokenUsageDay{{
				Date:   "2026-05-31",
				Tokens: TokenUsageTotal{Input: 1000, Output: 200, Total: 1200},
				Graph:  1200,
			}},
		},
	}

	out := RenderText(snapshot, RenderOptions{
		Now: time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
	})
	if !strings.Contains(out, "Token usage") {
		t.Fatalf("missing token usage section:\n%s", out)
	}
	if !strings.Contains(out, "Summary") {
		t.Fatalf("missing token usage summary:\n%s", out)
	}
	if !strings.Contains(out, "Monthly usage") {
		t.Fatalf("missing monthly usage section:\n%s", out)
	}
	if !strings.Contains(out, "Daily usage, last 30 days") {
		t.Fatalf("missing token time chart:\n%s", out)
	}
	if !strings.Contains(out, "Recent days") {
		t.Fatalf("missing recent days section:\n%s", out)
	}
	if strings.Contains(out, "#") {
		t.Fatalf("output should not use ASCII hash graphs:\n%s", out)
	}
	if strings.Contains(out, "Graph") {
		t.Fatalf("day summary should not include a graph column:\n%s", out)
	}
	for _, wanted := range []string{"Today", "This month", "All local", "2026-05", "1/31", "2026-05-31 1.2K"} {
		if !strings.Contains(out, wanted) {
			t.Fatalf("missing token usage detail %q:\n%s", wanted, out)
		}
	}
	for _, unwanted := range []string{"Aggregate", "Details", "me@example.com", "acct_123", "files", "events", "Sessions", "Events", "Cached", "Reason"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("pretty output includes unwanted detail %q:\n%s", unwanted, out)
		}
	}
}
