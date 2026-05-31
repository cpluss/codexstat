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

	out := RenderText(snapshot, RenderOptions{})
	if !strings.Contains(out, "Token usage") {
		t.Fatalf("missing token usage section:\n%s", out)
	}
	if !strings.Contains(out, "Tokens/day graph") {
		t.Fatalf("missing token time chart:\n%s", out)
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
	for _, unwanted := range []string{"Details", "me@example.com", "acct_123", "files", "events", "Sessions", "Events"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("pretty output includes unwanted detail %q:\n%s", unwanted, out)
		}
	}
}
