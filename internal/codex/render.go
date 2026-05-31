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
	lines = append(lines, sectionTitle(title, opts))
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

	lines = append(lines, renderDetails(snapshot, opts)...)
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
	renderedRows := make([]struct {
		usageRow
		reset string
	}, 0, len(rows))
	for _, row := range rows {
		reset := "-"
		if row.Window.ResetsAt != nil {
			reset = row.Window.ResetIn + " (" + row.Window.ResetsAt.Local().Format("Jan 02 15:04 MST") + ")"
		}
		renderedRows = append(renderedRows, struct {
			usageRow
			reset string
		}{usageRow: row, reset: reset})
	}

	tableRows := make([][]string, 0, len(renderedRows))
	for _, row := range renderedRows {
		tableRows = append(tableRows, []string{
			row.Title,
			formatPercent(row.Window.UsedPercent),
			formatPercent(row.Window.RemainingPercent),
			row.reset,
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

func renderDetails(snapshot *Snapshot, opts RenderOptions) []string {
	var lines []string
	lines = append(lines, sectionTitle("Details", opts))
	var rows [][]string
	if snapshot.Source != "" {
		rows = append(rows, []string{"Source", string(snapshot.Source)})
	}
	if snapshot.Credits != nil {
		rows = append(rows, []string{"Credits", formatCredits(*snapshot.Credits)})
	}
	if snapshot.Account != nil {
		if snapshot.Account.Email != "" {
			rows = append(rows, []string{"Account", snapshot.Account.Email})
		}
		if snapshot.Account.Plan != "" {
			rows = append(rows, []string{"Plan", displayPlan(snapshot.Account.Plan)})
		}
		if snapshot.Account.AccountID != "" {
			rows = append(rows, []string{"Account ID", snapshot.Account.AccountID})
		}
	}
	rows = append(rows, []string{"Updated", snapshot.UpdatedAt.Local().Format("2006-01-02 15:04:05 MST")})
	lines = append(lines, renderKeyValueTable(rows, opts)...)
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
