package codex

import (
	"fmt"
	"strings"
	"time"
)

type RenderOptions struct {
	Color bool
}

func RenderText(snapshot *Snapshot, opts RenderOptions) string {
	var lines []string
	title := "Codex stats"
	lines = append(lines, colorize(title, "1;36", opts.Color))
	lines = append(lines, strings.Repeat("-", len(title)))
	lines = append(lines, summaryLine(snapshot))
	lines = append(lines, "")

	rows := usageRows(snapshot)
	if len(rows) > 0 {
		lines = append(lines, renderUsageTable(rows, opts)...)
		lines = append(lines, "")
	}

	if snapshot.TokenUsage != nil {
		lines = append(lines, renderTokenUsageInline(*snapshot.TokenUsage, opts)...)
		lines = append(lines, "")
	}

	lines = append(lines, renderDetails(snapshot)...)
	for _, warning := range snapshot.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	return strings.Join(lines, "\n")
}

func usageRows(snapshot *Snapshot) []usageRow {
	var rows []usageRow
	if snapshot.Session != nil {
		rows = append(rows, usageRow{Title: "Session", Window: *snapshot.Session})
	}
	if snapshot.Weekly != nil {
		rows = append(rows, usageRow{Title: "Weekly", Window: *snapshot.Weekly})
	}
	for _, extra := range snapshot.Extra {
		rows = append(rows, usageRow{Title: extra.Title, Window: extra.Window})
	}
	return rows
}

type usageRow struct {
	Title  string
	Window Window
}

func renderUsageTable(rows []usageRow, opts RenderOptions) []string {
	titleWidth := len("Window")
	resetWidth := len("Reset")
	renderedRows := make([]struct {
		usageRow
		reset string
	}, 0, len(rows))
	for _, row := range rows {
		if len(row.Title) > titleWidth {
			titleWidth = len(row.Title)
		}
		reset := "-"
		if row.Window.ResetsAt != nil {
			reset = row.Window.ResetIn + " (" + row.Window.ResetsAt.Local().Format("Jan 02 15:04 MST") + ")"
		}
		if len(reset) > resetWidth {
			resetWidth = len(reset)
		}
		renderedRows = append(renderedRows, struct {
			usageRow
			reset string
		}{usageRow: row, reset: reset})
	}

	header := fmt.Sprintf(
		"%-*s  %6s  %6s  %-*s  %s",
		titleWidth,
		"Window",
		"Used",
		"Left",
		resetWidth,
		"Reset",
		"Usage",
	)
	lines := []string{
		colorize(header, "1;37", opts.Color),
		fmt.Sprintf(
			"%-*s  %6s  %6s  %-*s  %s",
			titleWidth,
			strings.Repeat("-", titleWidth),
			"------",
			"------",
			resetWidth,
			strings.Repeat("-", resetWidth),
			"--------------------",
		),
	}

	for _, row := range renderedRows {
		bar := usageBar(row.Window.UsedPercent, 20)
		if opts.Color {
			bar = colorizeBar(bar, row.Window.RemainingPercent)
		}
		lines = append(lines, fmt.Sprintf(
			"%-*s  %6s  %6s  %-*s  %s",
			titleWidth,
			row.Title,
			formatPercent(row.Window.UsedPercent),
			formatPercent(row.Window.RemainingPercent),
			resetWidth,
			row.reset,
			bar,
		))
	}
	return lines
}

func renderDetails(snapshot *Snapshot) []string {
	var lines []string
	lines = append(lines, "Details")
	lines = append(lines, "-------")
	if snapshot.Source != "" {
		lines = append(lines, "Source: "+string(snapshot.Source))
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
	lines = append(lines, "Updated: "+snapshot.UpdatedAt.Local().Format("2006-01-02 15:04:05 MST"))
	return lines
}

func summaryLine(snapshot *Snapshot) string {
	parts := []string{}
	if snapshot.Source != "" {
		parts = append(parts, "source "+string(snapshot.Source))
	}
	if snapshot.Account != nil && snapshot.Account.Email != "" {
		parts = append(parts, snapshot.Account.Email)
	}
	if snapshot.Account != nil && snapshot.Account.Plan != "" {
		parts = append(parts, displayPlan(snapshot.Account.Plan))
	}
	if snapshot.UpdatedAt.IsZero() {
		parts = append(parts, "updated unknown")
	} else {
		parts = append(parts, "updated "+snapshot.UpdatedAt.Local().Format("Jan 02 15:04 MST"))
	}
	return strings.Join(parts, " | ")
}

func usageBar(percent float64, width int) string {
	percent = clamp(percent, 0, 100)
	filled := int((percent / 100) * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	if percent > 0 && filled == 0 {
		filled = 1
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
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

func colorize(text, code string, enabled bool) string {
	if !enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func colorizeBar(bar string, remainingPercent float64) string {
	switch {
	case remainingPercent < 10:
		return colorize(bar, "31", true)
	case remainingPercent < 25:
		return colorize(bar, "33", true)
	default:
		return colorize(bar, "32", true)
	}
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
