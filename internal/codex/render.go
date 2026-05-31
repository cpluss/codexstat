package codex

import (
	"fmt"
	"strings"
	"time"
)

func RenderText(snapshot *Snapshot) string {
	var lines []string
	header := "Codex"
	if snapshot.Source != "" {
		header += " (" + string(snapshot.Source) + ")"
	}
	lines = append(lines, "== "+header+" ==")

	if snapshot.Session != nil {
		lines = appendWindow(lines, "Session", *snapshot.Session)
	}
	if snapshot.Weekly != nil {
		lines = appendWindow(lines, "Weekly", *snapshot.Weekly)
	}
	for _, extra := range snapshot.Extra {
		lines = appendWindow(lines, extra.Title, extra.Window)
	}
	if snapshot.Credits != nil {
		lines = append(lines, "Credits: "+formatCredits(*snapshot.Credits))
	}
	if snapshot.Account != nil {
		if snapshot.Account.Email != "" {
			lines = append(lines, "Account: "+snapshot.Account.Email)
		}
		if snapshot.Account.Plan != "" {
			lines = append(lines, "Plan: "+displayPlan(snapshot.Account.Plan))
		}
		if snapshot.Account.AccountID != "" {
			lines = append(lines, "Account ID: "+snapshot.Account.AccountID)
		}
	}
	for _, warning := range snapshot.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	lines = append(lines, "Updated: "+snapshot.UpdatedAt.Local().Format("2006-01-02 15:04:05 MST"))
	return strings.Join(lines, "\n")
}

func appendWindow(lines []string, title string, window Window) []string {
	lines = append(lines, fmt.Sprintf(
		"%s: %s left (%s used) %s",
		title,
		formatPercent(window.RemainingPercent),
		formatPercent(window.UsedPercent),
		usageBar(window.RemainingPercent),
	))
	if window.ResetsAt != nil {
		lines = append(lines, "  resets "+window.ResetIn+" at "+window.ResetsAt.Local().Format("2006-01-02 15:04 MST"))
	}
	return lines
}

func usageBar(remaining float64) string {
	const width = 12
	remaining = clamp(remaining, 0, 100)
	filled := int((remaining / 100) * width)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat("-", width-filled) + "]"
}

func formatPercent(value float64) string {
	if value == float64(int(value)) {
		return fmt.Sprintf("%d%%", int(value))
	}
	return fmt.Sprintf("%.1f%%", value)
}

func formatCredits(credits Credits) string {
	if credits.Unlimited {
		return "unlimited"
	}
	if credits.Balance == nil {
		if credits.HasCredits {
			return "available"
		}
		return "not available"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", *credits.Balance), "0"), ".")
}

func displayPlan(plan string) string {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return ""
	}
	parts := strings.FieldsFunc(plan, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func resetCountdown(reset time.Time, now time.Time) string {
	diff := reset.Sub(now)
	if diff <= 0 {
		return "now"
	}
	if diff < time.Minute {
		return "in <1m"
	}
	days := int(diff.Hours()) / 24
	hours := int(diff.Hours()) % 24
	minutes := int(diff.Minutes()) % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 && days == 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "now"
	}
	return "in " + strings.Join(parts, " ")
}
