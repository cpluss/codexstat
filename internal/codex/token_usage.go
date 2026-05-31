package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TokenUsageQuery struct {
	Days      int       `json:"days"`
	Metric    string    `json:"metric"`
	Now       time.Time `json:"now"`
	CodexHome string    `json:"codex_home,omitempty"`
	Env       map[string]string
}

const DefaultTokenUsageDays = 7

type TokenUsageReport struct {
	Metric        string          `json:"metric"`
	Since         string          `json:"since"`
	Until         string          `json:"until"`
	Roots         []string        `json:"roots"`
	FilesScanned  int             `json:"files_scanned"`
	EventsScanned int             `json:"events_scanned"`
	Total         TokenUsageTotal `json:"total"`
	Days          []TokenUsageDay `json:"days"`
}

type TokenUsageDay struct {
	Date     string          `json:"date"`
	Sessions int             `json:"sessions"`
	Events   int             `json:"events"`
	Tokens   TokenUsageTotal `json:"tokens"`
	Graph    int64           `json:"graph"`
}

type TokenUsageTotal struct {
	Input     int64 `json:"input"`
	Cached    int64 `json:"cached"`
	Output    int64 `json:"output"`
	Reasoning int64 `json:"reasoning"`
	Total     int64 `json:"total"`
}

func IsTokenHistoryMetric(metric string) bool {
	switch normalizeTokenMetric(metric) {
	case "tokens", "input", "cached", "output", "reasoning":
		return true
	default:
		return false
	}
}

func BuildTokenUsageReport(query TokenUsageQuery) (TokenUsageReport, error) {
	metric := normalizeTokenMetric(query.Metric)
	if !IsTokenHistoryMetric(metric) {
		return TokenUsageReport{}, fmt.Errorf("unknown token metric %q", query.Metric)
	}
	if query.Days <= 0 {
		query.Days = DefaultTokenUsageDays
	}
	if query.Now.IsZero() {
		query.Now = time.Now()
	}
	if query.Env == nil {
		query.Env = environMap()
	}

	today := startOfLocalDay(query.Now.Local())
	since := today.AddDate(0, 0, -(query.Days - 1))
	until := today.AddDate(0, 0, 1)
	scanSince := since.AddDate(0, 0, -1)
	scanUntil := until.AddDate(0, 0, 1)

	dayMap := make(map[string]*tokenUsageDayBuilder)
	for day := since; day.Before(until); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		dayMap[key] = &tokenUsageDayBuilder{
			day:      TokenUsageDay{Date: key},
			sessions: make(map[string]bool),
		}
	}

	roots, err := codexSessionRoots(query)
	if err != nil {
		return TokenUsageReport{}, err
	}
	files, err := listCodexTokenFiles(roots, scanSince, scanUntil)
	if err != nil {
		return TokenUsageReport{}, err
	}

	seenBase := make(map[string]bool)
	eventsScanned := 0
	filesScanned := 0
	for _, file := range files {
		base := filepath.Base(file)
		if seenBase[base] {
			continue
		}
		seenBase[base] = true
		fileEvents, err := scanCodexTokenFile(file, since, until, dayMap)
		if err != nil {
			return TokenUsageReport{}, err
		}
		if fileEvents > 0 {
			filesScanned++
			eventsScanned += fileEvents
		}
	}

	days := make([]TokenUsageDay, 0, len(dayMap))
	var total TokenUsageTotal
	for _, builder := range dayMap {
		builder.day.Sessions = len(builder.sessions)
		builder.day.Graph = tokenMetricValue(builder.day.Tokens, metric)
		total.add(builder.day.Tokens)
		days = append(days, builder.day)
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Date < days[j].Date
	})

	rootStrings := make([]string, 0, len(roots))
	for _, root := range roots {
		rootStrings = append(rootStrings, root)
	}

	return TokenUsageReport{
		Metric:        metric,
		Since:         since.Format("2006-01-02"),
		Until:         until.AddDate(0, 0, -1).Format("2006-01-02"),
		Roots:         rootStrings,
		FilesScanned:  filesScanned,
		EventsScanned: eventsScanned,
		Total:         total,
		Days:          days,
	}, nil
}

type tokenUsageDayBuilder struct {
	day      TokenUsageDay
	sessions map[string]bool
}

func codexSessionRoots(query TokenUsageQuery) ([]string, error) {
	root, err := codexHome(Options{
		CodexHome: query.CodexHome,
		Env:       query.Env,
	})
	if err != nil {
		return nil, err
	}
	return []string{
		filepath.Join(root, "sessions"),
		filepath.Join(root, "archived_sessions"),
	}, nil
}

func listCodexTokenFiles(roots []string, scanSince, scanUntil time.Time) ([]string, error) {
	scanSinceKey := scanSince.Format("2006-01-02")
	scanUntilKey := scanUntil.AddDate(0, 0, -1).Format("2006-01-02")
	var files []string
	for _, root := range roots {
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".jsonl" {
				return nil
			}
			if day := dateKeyFromCodexSessionPath(path); day != "" {
				if day < scanSinceKey || day > scanUntilKey {
					return nil
				}
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func dateKeyFromCodexSessionPath(path string) string {
	base := filepath.Base(path)
	idx := strings.Index(base, "rollout-")
	if idx >= 0 {
		start := idx + len("rollout-")
		if len(base) >= start+10 {
			candidate := base[start : start+10]
			if isDateKey(candidate) {
				return candidate
			}
		}
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 0; i+2 < len(parts); i++ {
		candidate := parts[i] + "-" + parts[i+1] + "-" + parts[i+2]
		if isDateKey(candidate) {
			return candidate
		}
	}
	return ""
}

func isDateKey(value string) bool {
	if len(value) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func scanCodexTokenFile(path string, since, until time.Time, days map[string]*tokenUsageDayBuilder) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 128*1024)
	sessionKey := filepath.Base(path)
	var previous *TokenUsageTotal
	events := 0

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if bytes.Contains(line, []byte(`"token_count"`)) {
				counted, err := scanCodexTokenLine(bytes.TrimSpace(line), sessionKey, since, until, days, &previous)
				if err != nil {
					return events, fmt.Errorf("%s: %w", path, err)
				}
				if counted {
					events++
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return events, readErr
		}
	}
	return events, nil
}

func scanCodexTokenLine(
	line []byte,
	sessionKey string,
	since time.Time,
	until time.Time,
	days map[string]*tokenUsageDayBuilder,
	previous **TokenUsageTotal,
) (bool, error) {
	var event codexTokenEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return false, nil
	}
	if event.Type != "event_msg" || event.Payload.Type != "token_count" || event.Payload.Info == nil {
		return false, nil
	}
	timestamp, err := parseCodexTimestamp(event.Timestamp)
	if err != nil {
		return false, nil
	}

	total := event.Payload.Info.TotalTokenUsage.total()
	last := event.Payload.Info.LastTokenUsage.total()
	delta, ok := tokenDelta(last, total, previous)
	if !ok || delta.empty() {
		return false, nil
	}

	localTime := timestamp.Local()
	if localTime.Before(since) || !localTime.Before(until) {
		return false, nil
	}
	dayKey := startOfLocalDay(localTime).Format("2006-01-02")
	day := days[dayKey]
	if day == nil {
		return false, nil
	}
	day.day.Events++
	day.day.Tokens.add(delta)
	day.sessions[sessionKey] = true
	return true, nil
}

func tokenDelta(last *TokenUsageTotal, total *TokenUsageTotal, previous **TokenUsageTotal) (TokenUsageTotal, bool) {
	if last != nil {
		delta := *last
		if total != nil {
			if *previous != nil {
				totalDelta := total.delta(**previous)
				if totalDelta.nonNegativeLTE(delta) {
					delta = totalDelta
				}
			}
			next := *total
			*previous = &next
		} else {
			base := TokenUsageTotal{}
			if *previous != nil {
				base = **previous
			}
			next := base
			next.add(delta)
			*previous = &next
		}
		return delta, true
	}
	if total != nil {
		delta := *total
		if *previous != nil {
			delta = total.delta(**previous)
		}
		next := *total
		*previous = &next
		return delta, true
	}
	return TokenUsageTotal{}, false
}

type codexTokenEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type string          `json:"type"`
		Info *codexTokenInfo `json:"info"`
	} `json:"payload"`
}

type codexTokenInfo struct {
	TotalTokenUsage *codexRawTokenUsage `json:"total_token_usage"`
	LastTokenUsage  *codexRawTokenUsage `json:"last_token_usage"`
}

type codexRawTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

func (usage *codexRawTokenUsage) total() *TokenUsageTotal {
	if usage == nil {
		return nil
	}
	cached := usage.CachedInputTokens
	if cached == 0 {
		cached = usage.CacheReadInputTokens
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.InputTokens + usage.OutputTokens
	}
	out := TokenUsageTotal{
		Input:     maxInt64(0, usage.InputTokens),
		Cached:    maxInt64(0, cached),
		Output:    maxInt64(0, usage.OutputTokens),
		Reasoning: maxInt64(0, usage.ReasoningOutputTokens),
		Total:     maxInt64(0, total),
	}
	if out.Cached > out.Input {
		out.Cached = out.Input
	}
	return &out
}

func parseCodexTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("missing timestamp")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
}

func (total *TokenUsageTotal) add(other TokenUsageTotal) {
	total.Input += other.Input
	total.Cached += other.Cached
	total.Output += other.Output
	total.Reasoning += other.Reasoning
	total.Total += other.Total
}

func (total TokenUsageTotal) delta(previous TokenUsageTotal) TokenUsageTotal {
	return TokenUsageTotal{
		Input:     maxInt64(0, total.Input-previous.Input),
		Cached:    maxInt64(0, total.Cached-previous.Cached),
		Output:    maxInt64(0, total.Output-previous.Output),
		Reasoning: maxInt64(0, total.Reasoning-previous.Reasoning),
		Total:     maxInt64(0, total.Total-previous.Total),
	}
}

func (total TokenUsageTotal) empty() bool {
	return total.Input == 0 && total.Cached == 0 && total.Output == 0 && total.Reasoning == 0 && total.Total == 0
}

func (total TokenUsageTotal) nonNegativeLTE(other TokenUsageTotal) bool {
	return total.Input <= other.Input &&
		total.Cached <= other.Cached &&
		total.Output <= other.Output &&
		total.Reasoning <= other.Reasoning &&
		total.Total <= other.Total
}

func tokenMetricValue(total TokenUsageTotal, metric string) int64 {
	switch normalizeTokenMetric(metric) {
	case "input":
		return total.Input
	case "cached":
		return total.Cached
	case "output":
		return total.Output
	case "reasoning":
		return total.Reasoning
	default:
		return total.Total
	}
}

func normalizeTokenMetric(metric string) string {
	metric = strings.ToLower(strings.TrimSpace(metric))
	switch metric {
	case "", "token", "total", "totals", "tokens":
		return "tokens"
	default:
		return metric
	}
}

func maxInt64(minimum, value int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}
