package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveUsageURLDefaultsToWhamUsage(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveUsageURL(Options{CodexHome: dir, Env: map[string]string{"HOME": dir}})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://chatgpt.com/backend-api/wham/usage"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveUsageURLUsesConfigBase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`chatgpt_base_url = "https://example.com/base"`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveUsageURL(Options{CodexHome: dir, Env: map[string]string{"HOME": dir}})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/base/api/codex/usage"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSnapshotFromUsageResponseMapsWindowsCreditsAndExtras(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	data := []byte(`{
	  "plan_type": "pro",
	  "rate_limit": {
	    "primary_window": {"used_percent": 25, "reset_at": 1800003600, "limit_window_seconds": 18000},
	    "secondary_window": {"used_percent": 50, "reset_at": 1800604800, "limit_window_seconds": 604800}
	  },
	  "credits": {"has_credits": true, "unlimited": false, "balance": "123.45"},
	  "additional_rate_limits": [{
	    "limit_name": "GPT-5.3-Codex-Spark",
	    "metered_feature": "spark",
	    "rate_limit": {
	      "primary_window": {"used_percent": 10, "reset_at": 1800003600, "limit_window_seconds": 18000},
	      "secondary_window": {"used_percent": 20, "reset_at": 1800604800, "limit_window_seconds": 604800}
	    }
	  }]
	}`)
	var payload usageResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	snapshot := snapshotFromUsageResponse(payload, credentials{}, now)
	if snapshot.Session == nil || snapshot.Session.RemainingPercent != 75 {
		t.Fatalf("unexpected session: %#v", snapshot.Session)
	}
	if snapshot.Weekly == nil || snapshot.Weekly.RemainingPercent != 50 {
		t.Fatalf("unexpected weekly: %#v", snapshot.Weekly)
	}
	if snapshot.Credits == nil || snapshot.Credits.Balance == nil || *snapshot.Credits.Balance != 123.45 {
		t.Fatalf("unexpected credits: %#v", snapshot.Credits)
	}
	if len(snapshot.Extra) != 2 || snapshot.Extra[0].ID != "codex-spark" || snapshot.Extra[1].ID != "codex-spark-weekly" {
		t.Fatalf("unexpected extra windows: %#v", snapshot.Extra)
	}
}
