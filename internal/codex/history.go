package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/barchart"
)

const historyVersion = 1

type HistoryRecord struct {
	Version    int       `json:"version"`
	CapturedAt time.Time `json:"captured_at"`
	Snapshot   Snapshot  `json:"snapshot"`
}

func RenderTokenUsage(report TokenUsageReport, opts RenderOptions) string {
	var lines []string
	lines = append(lines, sectionTitle("Codex token usage", opts))
	lines = append(lines, renderTokenUsageRows(report, opts)...)
	return strings.Join(lines, "\n")
}

func renderTokenUsageInline(report TokenUsageReport, opts RenderOptions) []string {
	lines := []string{sectionTitle("Token usage", opts)}
	lines = append(lines, renderTokenUsageRows(report, opts)...)
	return lines
}

func renderTokenUsageRows(report TokenUsageReport, opts RenderOptions) []string {
	var lines []string
	lines = append(lines, renderTokenUsageTimeChart(report, opts)...)
	if len(lines) > 0 {
		lines = append(lines, "")
	}

	rows := make([][]string, 0, len(report.Days)+2)
	for _, day := range report.Days {
		rows = append(rows, []string{
			day.Date,
			formatTokenCount(day.Tokens.Input),
			formatTokenCount(day.Tokens.Cached),
			formatTokenCount(day.Tokens.Output),
			formatTokenCount(day.Tokens.Reasoning),
			formatTokenCount(day.Tokens.Total),
		})
	}
	if len(report.Days) > 0 {
		rows = append(rows, tableSeparatorRow(6))
	}
	rows = append(rows, []string{
		"Aggregate",
		formatTokenCount(report.Total.Input),
		formatTokenCount(report.Total.Cached),
		formatTokenCount(report.Total.Output),
		formatTokenCount(report.Total.Reasoning),
		formatTokenCount(report.Total.Total),
	})
	lines = append(lines, renderPrettyTable(
		[]string{"Date", "Input", "Cached", "Output", "Reason", "Total"},
		rows,
		opts,
		1,
		2,
		3,
		4,
		5,
	)...)
	return lines
}

func renderTokenUsageTimeChart(report TokenUsageReport, opts RenderOptions) []string {
	const chartHeight = 9

	if len(report.Days) == 0 {
		return nil
	}

	maxGraph := int64(0)
	for _, day := range report.Days {
		if day.Graph > maxGraph {
			maxGraph = day.Graph
		}
	}

	title := titleCaseASCII(report.Metric) + "/day graph"
	lines := []string{sectionTitle(title, opts)}
	if maxGraph <= 0 {
		lines = append(lines, mutedText("no token usage in range", opts))
		return lines
	}

	barData := make([]barchart.BarData, 0, len(report.Days))
	for index, day := range report.Days {
		barData = append(barData, barchart.BarData{
			Label: chartDayLabel(day.Date, index, len(report.Days)),
			Values: []barchart.BarValue{{
				Name:  report.Metric,
				Value: float64(day.Graph),
				Style: chartBarStyle(opts),
			}},
		})
	}

	chart := barchart.New(
		tokenUsageChartWidth(len(report.Days)),
		chartHeight,
		barchart.WithDataSet(barData),
		barchart.WithMaxValue(float64(maxGraph)),
		barchart.WithNoAutoMaxValue(),
		barchart.WithBarGap(tokenUsageChartGap(len(report.Days))),
		barchart.WithStyles(chartAxisStyle(opts), chartLabelStyle(opts)),
	)
	chart.Draw()
	lines = append(lines, splitRenderedLines(chart.View())...)
	lines = append(lines, mutedText(fmt.Sprintf("max/day %s", formatTokenCount(maxGraph)), opts))
	return lines
}

func tokenUsageChartWidth(days int) int {
	switch {
	case days <= 8:
		return 24
	case days <= 31:
		return days * 3
	case days <= 90:
		return days * 2
	default:
		return days
	}
}

func tokenUsageChartGap(days int) int {
	if days > 90 {
		return 0
	}
	return 1
}

func chartDayLabel(date string, index, total int) string {
	if total <= 31 {
		return dayLabel(date)
	}
	step := total / 12
	if step < 1 {
		step = 1
	}
	if index%step == 0 || index == total-1 {
		return dayLabel(date)
	}
	return ""
}

func dayLabel(date string) string {
	if len(date) >= 10 {
		return date[8:10]
	}
	return date
}

func titleCaseASCII(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func formatTokenCount(value int64) string {
	switch {
	case value >= 1_000_000_000:
		return trimFloat(float64(value)/1_000_000_000) + "B"
	case value >= 1_000_000:
		return trimFloat(float64(value)/1_000_000) + "M"
	case value >= 1_000:
		return trimFloat(float64(value)/1_000) + "K"
	default:
		return fmt.Sprintf("%d", value)
	}
}

func trimFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", value), "0"), ".")
}

type HistoryQuery struct {
	Days   int       `json:"days"`
	Metric string    `json:"metric"`
	Now    time.Time `json:"now"`
	Path   string    `json:"path,omitempty"`
}

type HistoryReport struct {
	Path        string       `json:"path"`
	Metric      string       `json:"metric"`
	Since       string       `json:"since"`
	Until       string       `json:"until"`
	TotalRows   int          `json:"total_rows"`
	MatchedRows int          `json:"matched_rows"`
	Days        []HistoryDay `json:"days"`
}

type HistoryDay struct {
	Date            string   `json:"date"`
	Samples         int      `json:"samples"`
	SessionPeakUsed *float64 `json:"session_peak_used,omitempty"`
	SessionLastUsed *float64 `json:"session_last_used,omitempty"`
	WeeklyPeakUsed  *float64 `json:"weekly_peak_used,omitempty"`
	WeeklyLastUsed  *float64 `json:"weekly_last_used,omitempty"`
	CreditsLast     *float64 `json:"credits_last,omitempty"`
	SourceLast      Source   `json:"source_last,omitempty"`
	AccountLast     string   `json:"account_last,omitempty"`
	GraphValue      *float64 `json:"graph_value,omitempty"`
}

func DefaultHistoryPath(env map[string]string) (string, error) {
	if env == nil {
		env = environMap()
	}
	if path := strings.TrimSpace(env["CODEXSTAT_HISTORY"]); path != "" {
		return expandHome(path, env), nil
	}
	if base := strings.TrimSpace(env["XDG_STATE_HOME"]); base != "" {
		return filepath.Join(expandHome(base, env), "codexstat", "history.jsonl"), nil
	}
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "codexstat", "history.jsonl"), nil
	}
	return filepath.Join(home, ".local", "state", "codexstat", "history.jsonl"), nil
}

func RecordSnapshot(path string, snapshot *Snapshot) error {
	if snapshot == nil {
		return errors.New("nil snapshot")
	}
	if path == "" {
		return errors.New("missing history path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	record := HistoryRecord{
		Version:    historyVersion,
		CapturedAt: time.Now(),
		Snapshot:   *snapshot,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func LoadHistory(path string) ([]HistoryRecord, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []HistoryRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		record, err := parseHistoryLine([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func parseHistoryLine(data []byte) (HistoryRecord, error) {
	var record HistoryRecord
	if err := json.Unmarshal(data, &record); err == nil && record.Snapshot.Provider != "" {
		if record.CapturedAt.IsZero() {
			record.CapturedAt = record.Snapshot.UpdatedAt
		}
		return record, nil
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return HistoryRecord{}, err
	}
	if snapshot.Provider == "" {
		return HistoryRecord{}, errors.New("history line does not contain a snapshot")
	}
	capturedAt := snapshot.UpdatedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}
	return HistoryRecord{
		Version:    historyVersion,
		CapturedAt: capturedAt,
		Snapshot:   snapshot,
	}, nil
}

func BuildHistoryReport(records []HistoryRecord, query HistoryQuery) (HistoryReport, error) {
	metric := strings.ToLower(strings.TrimSpace(query.Metric))
	if metric == "" {
		metric = "weekly"
	}
	if metric != "weekly" && metric != "session" {
		return HistoryReport{}, fmt.Errorf("unknown quota history metric %q; expected weekly or session", query.Metric)
	}
	if query.Days <= 0 {
		query.Days = 7
	}
	if query.Now.IsZero() {
		query.Now = time.Now()
	}

	local := query.Now.Local()
	today := startOfLocalDay(local)
	since := today.AddDate(0, 0, -(query.Days - 1))
	until := today.AddDate(0, 0, 1)

	byDate := make(map[string]*HistoryDay)
	for day := since; day.Before(until); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		byDate[key] = &HistoryDay{Date: key}
	}

	matched := 0
	for _, record := range records {
		capturedAt := record.CapturedAt
		if capturedAt.IsZero() {
			capturedAt = record.Snapshot.UpdatedAt
		}
		if capturedAt.IsZero() {
			continue
		}
		localCaptured := capturedAt.Local()
		if localCaptured.Before(since) || !localCaptured.Before(until) {
			continue
		}
		key := startOfLocalDay(localCaptured).Format("2006-01-02")
		day := byDate[key]
		if day == nil {
			continue
		}
		matched++
		applyHistorySample(day, record.Snapshot)
	}

	days := make([]HistoryDay, 0, len(byDate))
	for _, day := range byDate {
		if metric == "session" {
			day.GraphValue = day.SessionPeakUsed
		} else {
			day.GraphValue = firstFloatPtr(day.WeeklyLastUsed, day.WeeklyPeakUsed)
		}
		days = append(days, *day)
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Date < days[j].Date
	})

	return HistoryReport{
		Path:        query.Path,
		Metric:      metric,
		Since:       since.Format("2006-01-02"),
		Until:       until.AddDate(0, 0, -1).Format("2006-01-02"),
		TotalRows:   len(records),
		MatchedRows: matched,
		Days:        days,
	}, nil
}

func applyHistorySample(day *HistoryDay, snapshot Snapshot) {
	day.Samples++
	day.SourceLast = snapshot.Source
	if snapshot.Account != nil {
		day.AccountLast = snapshot.Account.Email
	}
	if snapshot.Session != nil {
		used := snapshot.Session.UsedPercent
		day.SessionLastUsed = ptr(used)
		day.SessionPeakUsed = maxFloatPtr(day.SessionPeakUsed, used)
	}
	if snapshot.Weekly != nil {
		used := snapshot.Weekly.UsedPercent
		day.WeeklyLastUsed = ptr(used)
		day.WeeklyPeakUsed = maxFloatPtr(day.WeeklyPeakUsed, used)
	}
	if snapshot.Credits != nil && snapshot.Credits.Balance != nil {
		day.CreditsLast = ptr(*snapshot.Credits.Balance)
	}
}

func startOfLocalDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func maxFloatPtr(current *float64, next float64) *float64 {
	if current == nil || next > *current {
		return ptr(next)
	}
	return current
}

func firstFloatPtr(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func RenderHistory(report HistoryReport, opts RenderOptions) string {
	var lines []string
	lines = append(lines, sectionTitle("Codex quota history", opts))

	rows := make([][]string, 0, len(report.Days))
	for _, day := range report.Days {
		rows = append(rows, []string{
			day.Date,
			formatOptionalPercent(day.SessionPeakUsed),
			formatOptionalPercent(day.WeeklyLastUsed),
		})
	}
	lines = append(lines, renderPrettyTable(
		[]string{"Date", "Session peak", "Weekly last"},
		rows,
		opts,
		1,
		2,
	)...)
	return strings.Join(lines, "\n")
}

func formatOptionalPercent(value *float64) string {
	if value == nil {
		return "-"
	}
	return formatPercent(*value)
}
