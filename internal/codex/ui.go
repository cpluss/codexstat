package codex

import (
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
)

const (
	colorPrimary = "#7dd3fc"
	colorAccent  = "#34d399"
	colorMuted   = "#8b949e"
	colorBorder  = "#30363d"
	colorWarn    = "#fbbf24"
	colorDanger  = "#f87171"

	tableSeparatorCell = "────────"
)

func sectionTitle(title string, opts RenderOptions) string {
	if !opts.Color {
		return title
	}
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorPrimary)).
		Render(title)
}

func mutedText(text string, opts RenderOptions) string {
	if !opts.Color {
		return text
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Render(text)
}

func renderPrettyTable(headers []string, rows [][]string, opts RenderOptions, rightAlignedCols ...int) []string {
	rightAligned := make(map[int]bool, len(rightAlignedCols))
	for _, col := range rightAlignedCols {
		rightAligned[col] = true
	}

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(tableBorderStyle(opts)).
		BorderHeader(true).
		BorderColumn(true).
		Wrap(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if rightAligned[col] {
				style = style.Align(lipgloss.Right)
			}
			if !opts.Color {
				return style
			}
			switch {
			case row == table.HeaderRow:
				return style.Bold(true).Foreground(lipgloss.Color(colorPrimary))
			case isTableSeparatorRow(rows, row):
				return style.Foreground(lipgloss.Color(colorBorder))
			case isAggregateRow(rows, row):
				return style.Bold(true).Foreground(lipgloss.Color(colorAccent))
			case row%2 == 0:
				return style.Foreground(lipgloss.Color("#c9d1d9"))
			default:
				return style.Foreground(lipgloss.Color("#f0f6fc"))
			}
		})

	return splitRenderedLines(t.Render())
}

func tableSeparatorRow(columns int) []string {
	row := make([]string, columns)
	for i := range row {
		row[i] = tableSeparatorCell
	}
	return row
}

func isTableSeparatorRow(rows [][]string, row int) bool {
	return row >= 0 && row < len(rows) && len(rows[row]) > 0 && rows[row][0] == tableSeparatorCell
}

func isAggregateRow(rows [][]string, row int) bool {
	return row >= 0 && row < len(rows) && len(rows[row]) > 0 && rows[row][0] == "Aggregate"
}

func splitRenderedLines(value string) []string {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return nil
	}
	raw := strings.Split(value, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		lines = append(lines, strings.TrimRight(line, " \t"))
	}
	return lines
}

func tableBorderStyle(opts RenderOptions) lipgloss.Style {
	if !opts.Color {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorBorder))
}

func chartAxisStyle(opts RenderOptions) lipgloss.Style {
	if !opts.Color {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
}

func chartLabelStyle(opts RenderOptions) lipgloss.Style {
	if !opts.Color {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPrimary))
}

func chartBarStyle(opts RenderOptions) lipgloss.Style {
	if !opts.Color {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Background(lipgloss.Color(colorAccent))
}

func progressBar(percent float64, width int, opts RenderOptions, fillColor string) string {
	percent = clamp(percent, 0, 100)
	profile := termenv.Ascii
	if opts.Color {
		profile = termenv.ColorProfile()
	}
	bar := progress.New(
		progress.WithWidth(width),
		progress.WithoutPercentage(),
		progress.WithFillCharacters('█', '░'),
		progress.WithColorProfile(profile),
	)
	if opts.Color {
		bar.FullColor = fillColor
		bar.EmptyColor = colorBorder
	}
	return bar.ViewAs(percent / 100)
}

func healthFillColor(remainingPercent float64) string {
	switch {
	case remainingPercent < 10:
		return colorDanger
	case remainingPercent < 25:
		return colorWarn
	default:
		return colorAccent
	}
}
