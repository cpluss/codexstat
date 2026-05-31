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
)

const historyVersion = 1

type HistoryRecord struct {
	Version    int       `json:"version"`
	CapturedAt time.Time `json:"captured_at"`
	Snapshot   Snapshot  `json:"snapshot"`
}

func RenderTokenUsage(report TokenUsageReport, opts RenderOptions) string {
	var lines []string
	title := "Codex token usage"
	lines = append(lines, colorize(title, "1;36", opts.Color))
	lines = append(lines, strings.Repeat("-", len(title)))
	lines = append(lines, fmt.Sprintf(
		"range %s..%s | metric %s | files %d | events %d | total %s",
		report.Since,
		report.Until,
		report.Metric,
		report.FilesScanned,
		report.EventsScanned,
		formatTokenCount(report.Total.Total),
	))
	if len(report.Roots) > 0 {
		lines = append(lines, "roots "+strings.Join(report.Roots, ", "))
	}
	lines = append(lines, "")

	header := fmt.Sprintf(
		"%-10s  %8s  %8s  %9s  %9s  %9s  %9s  %9s  %s",
		"Date",
		"Sessions",
		"Events",
		"Input",
		"Cached",
		"Output",
		"Reason",
		"Total",
		"Graph",
	)
	lines = append(lines, colorize(header, "1;37", opts.Color))
	lines = append(lines, fmt.Sprintf(
		"%-10s  %8s  %8s  %9s  %9s  %9s  %9s  %9s  %s",
		"----------",
		"--------",
		"--------",
		"---------",
		"---------",
		"---------",
		"---------",
		"---------",
		"--------------------",
	))

	maxGraph := int64(0)
	for _, day := range report.Days {
		if day.Graph > maxGraph {
			maxGraph = day.Graph
		}
	}
	for _, day := range report.Days {
		lines = append(lines, fmt.Sprintf(
			"%-10s  %8d  %8d  %9s  %9s  %9s  %9s  %9s  %s",
			day.Date,
			day.Sessions,
			day.Events,
			formatTokenCount(day.Tokens.Input),
			formatTokenCount(day.Tokens.Cached),
			formatTokenCount(day.Tokens.Output),
			formatTokenCount(day.Tokens.Reasoning),
			formatTokenCount(day.Tokens.Total),
			tokenUsageGraph(day.Graph, maxGraph, opts),
		))
	}
	return strings.Join(lines, "\n")
}

func tokenUsageGraph(value int64, maxValue int64, opts RenderOptions) string {
	if value <= 0 || maxValue <= 0 {
		return "[--------------------] -"
	}
	percent := float64(value) / float64(maxValue) * 100
	bar := usageBar(percent, 20)
	if opts.Color {
		bar = colorize(bar, "32", true)
	}
	return bar + " " + formatTokenCount(value)
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
	title := "Codex quota history"
	lines = append(lines, colorize(title, "1;36", opts.Color))
	lines = append(lines, strings.Repeat("-", len(title)))
	lines = append(lines, fmt.Sprintf(
		"range %s..%s | metric %s | samples %d/%d",
		report.Since,
		report.Until,
		report.Metric,
		report.MatchedRows,
		report.TotalRows,
	))
	if report.Path != "" {
		lines = append(lines, "file "+report.Path)
	}
	lines = append(lines, "")

	header := fmt.Sprintf(
		"%-10s  %7s  %12s  %11s  %8s  %s",
		"Date",
		"Samples",
		"Session peak",
		"Weekly last",
		"Credits",
		"Graph",
	)
	lines = append(lines, colorize(header, "1;37", opts.Color))
	lines = append(lines, fmt.Sprintf(
		"%-10s  %7s  %12s  %11s  %8s  %s",
		"----------",
		"-------",
		"------------",
		"-----------",
		"--------",
		"--------------------",
	))
	for _, day := range report.Days {
		graph := historyGraph(day.GraphValue, opts)
		lines = append(lines, fmt.Sprintf(
			"%-10s  %7d  %12s  %11s  %8s  %s",
			day.Date,
			day.Samples,
			formatOptionalPercent(day.SessionPeakUsed),
			formatOptionalPercent(day.WeeklyLastUsed),
			formatOptionalCredits(day.CreditsLast),
			graph,
		))
	}
	return strings.Join(lines, "\n")
}

func historyGraph(value *float64, opts RenderOptions) string {
	if value == nil {
		return "[--------------------] -"
	}
	bar := usageBar(*value, 20)
	if opts.Color {
		bar = colorizeBar(bar, 100-*value)
	}
	return bar + " " + formatPercent(*value)
}

func formatOptionalPercent(value *float64) string {
	if value == nil {
		return "-"
	}
	return formatPercent(*value)
}

func formatOptionalCredits(value *float64) string {
	if value == nil {
		return "-"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", *value), "0"), ".")
}
