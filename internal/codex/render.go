package codex

import (
	"fmt"
	"strings"
	"time"
)

type RenderOptions struct {
	Color bool
	Now   time.Time
}

func RenderText(snapshot *Snapshot, opts RenderOptions) string {
	var lines []string
	lines = append(lines, sectionTitle("Codex stats", opts))

	rows := usageRows(snapshot)
	if len(rows) > 0 {
		lines = append(lines, renderUsageTable(rows, opts)...)
	}

	if snapshot.TokenUsage != nil {
		if len(lines) > 1 {
			lines = append(lines, "")
		}
		lines = append(lines, renderTokenUsageInline(*snapshot.TokenUsage, opts)...)
	}

	if len(snapshot.Warnings) > 0 {
		lines = append(lines, "")
		for _, warning := range snapshot.Warnings {
			lines = append(lines, "Warning: "+warning)
		}
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
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		reset := "-"
		if row.Window.ResetsAt != nil {
			reset = row.Window.ResetIn + " (" + row.Window.ResetsAt.Local().Format("Jan 02 15:04 MST") + ")"
		}
		tableRows = append(tableRows, []string{
			row.Title,
			formatPercent(row.Window.UsedPercent),
			formatPercent(row.Window.RemainingPercent),
			reset,
			progressBar(row.Window.UsedPercent, 20, opts, healthFillColor(row.Window.RemainingPercent)),
		})
	}
	return renderPrettyTable(
		[]string{"Window", "Used", "Left", "Reset", "Usage"},
		tableRows,
		opts,
		1,
		2,
	)
}

func formatPercent(value float64) string {
	if value == float64(int(value)) {
		return fmt.Sprintf("%d%%", int(value))
	}
	return fmt.Sprintf("%.1f%%", value)
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
