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
	if !strings.Contains(out, "Token usage (2026-05-25..2026-05-31)") {
		t.Fatalf("missing token usage section:\n%s", out)
	}
	if !strings.Contains(out, "Tokens/day graph") {
		t.Fatalf("missing token time chart:\n%s", out)
	}
	if strings.Contains(out, "#") {
		t.Fatalf("output should not use ASCII hash graphs:\n%s", out)
	}
	if !strings.Contains(out, "████████████████████ 1.2K") {
		t.Fatalf("missing token graph:\n%s", out)
	}
	if !strings.Contains(out, "Aggregate") {
		t.Fatalf("missing aggregate row:\n%s", out)
	}
}
