package codex

import "testing"

func TestNormalizeWindowsSwapsWeeklyPrimary(t *testing.T) {
	weeklyMinutes := 10080
	sessionMinutes := 300
	weekly := &Window{WindowMinutes: &weeklyMinutes}
	session := &Window{WindowMinutes: &sessionMinutes}

	primary, secondary := normalizeWindows(weekly, session)
	if primary != session || secondary != weekly {
		t.Fatalf("expected session primary and weekly secondary")
	}
}
