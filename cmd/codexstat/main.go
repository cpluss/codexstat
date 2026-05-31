package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"codexstat/internal/codex"
)

const version = "0.4.3"

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "history", "hist":
			runHistory(args[1:])
			return
		case "tokens", "token":
			runHistory(append([]string{"--metric", "tokens"}, args[1:]...))
			return
		case "help":
			if len(args) > 1 && (args[1] == "history" || args[1] == "hist") {
				runHistory([]string{"-h"})
				return
			}
			if len(args) > 1 && (args[1] == "tokens" || args[1] == "token") {
				runHistory([]string{"-h"})
				return
			}
			runNow([]string{"-h"})
			return
		}
	}

	runNow(args)
}

func runNow(args []string) {
	if len(args) > 0 && args[0] == "now" {
		args = args[1:]
	}

	flags := flag.NewFlagSet("codexstat", flag.ExitOnError)
	var (
		sourceFlag  = flags.String("source", "auto", "data source: auto, oauth, or cli")
		jsonFlag    = flags.Bool("json", false, "print JSON instead of text")
		prettyFlag  = flags.Bool("pretty", false, "pretty-print JSON output")
		noRefresh   = flags.Bool("no-refresh", false, "do not refresh stale OAuth tokens")
		codexHome   = flags.String("codex-home", "", "Codex home directory containing auth.json and config.toml")
		codexBin    = flags.String("codex-bin", "codex", "codex executable path or name for CLI fallback")
		timeout     = flags.Duration("timeout", 15*time.Second, "overall fetch timeout")
		historyFile = flags.String("history-file", "", "history JSONL file path")
		noRecord    = flags.Bool("no-record", false, "do not append this fetch to history")
		noColor     = flags.Bool("no-color", false, "disable ANSI color in text output")
		showVer     = flags.Bool("version", false, "print version and exit")
	)

	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [flags]\n", os.Args[0])
		fmt.Fprintf(flags.Output(), "       %s history [flags]\n\n", os.Args[0])
		fmt.Fprintln(flags.Output(), "Print current Codex account, credit, and rate-limit stats.")
		fmt.Fprintln(flags.Output(), "\nFlags:")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		exitErr(err, 2)
	}

	if *showVer {
		fmt.Println(version)
		return
	}

	source, err := codex.ParseSource(*sourceFlag)
	if err != nil {
		exitErr(err, 2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	snapshot, err := codex.Fetch(ctx, codex.Options{
		Source:    source,
		CodexHome: *codexHome,
		CodexBin:  *codexBin,
		Timeout:   *timeout,
		NoRefresh: *noRefresh,
	})
	if err != nil {
		exitErr(err, 1)
	}

	if !*noRecord {
		path := *historyFile
		if path == "" {
			var pathErr error
			path, pathErr = codex.DefaultHistoryPath(nil)
			if pathErr != nil {
				snapshot.Warnings = append(snapshot.Warnings, "history path unavailable: "+pathErr.Error())
			}
		}
		if path != "" {
			if err := codex.RecordSnapshot(path, snapshot); err != nil {
				snapshot.Warnings = append(snapshot.Warnings, "history write failed: "+err.Error())
			}
		}
	}

	tokenUsageQuery := codex.TokenUsageQuery{
		Days:      codex.DefaultTokenUsageDays,
		Metric:    "tokens",
		Now:       time.Now(),
		CodexHome: *codexHome,
	}
	tokenUsage, err := codex.BuildTokenUsageReport(tokenUsageQuery)
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "token usage unavailable: "+err.Error())
		tokenUsage = codex.EmptyTokenUsageReport(tokenUsageQuery)
	}
	snapshot.TokenUsage = &tokenUsage

	if *jsonFlag {
		writeJSON(snapshot, *prettyFlag)
		return
	}

	fmt.Println(codex.RenderText(snapshot, codex.RenderOptions{
		Color: shouldUseColor(*noColor),
	}))
}

func runHistory(args []string) {
	flags := historyFlagSet()
	var (
		days        = flags.Int("days", 7, "number of days to show")
		metric      = flags.String("metric", "tokens", "graph metric: tokens, input, cached, output, reasoning, weekly, or session")
		codexHome   = flags.String("codex-home", "", "Codex home directory containing sessions and archived_sessions")
		historyFile = flags.String("history-file", "", "quota snapshot history JSONL file path")
		jsonFlag    = flags.Bool("json", false, "print JSON instead of text")
		prettyFlag  = flags.Bool("pretty", false, "pretty-print JSON output")
		noColor     = flags.Bool("no-color", false, "disable ANSI color in text output")
	)
	if err := flags.Parse(args); err != nil {
		exitErr(err, 2)
	}

	if codex.IsTokenHistoryMetric(*metric) {
		report, err := codex.BuildTokenUsageReport(codex.TokenUsageQuery{
			Days:      *days,
			Metric:    *metric,
			Now:       time.Now(),
			CodexHome: *codexHome,
		})
		if err != nil {
			exitErr(err, 1)
		}
		if *jsonFlag {
			writeJSON(report, *prettyFlag)
			return
		}
		fmt.Println(codex.RenderTokenUsage(report, codex.RenderOptions{
			Color: shouldUseColor(*noColor),
		}))
		return
	}

	path := *historyFile
	if path == "" {
		var err error
		path, err = codex.DefaultHistoryPath(nil)
		if err != nil {
			exitErr(err, 1)
		}
	}

	records, err := codex.LoadHistory(path)
	if err != nil {
		exitErr(err, 1)
	}
	report, err := codex.BuildHistoryReport(records, codex.HistoryQuery{
		Days:   *days,
		Metric: *metric,
		Now:    time.Now(),
		Path:   path,
	})
	if err != nil {
		exitErr(err, 2)
	}

	if *jsonFlag {
		writeJSON(report, *prettyFlag)
		return
	}

	fmt.Println(codex.RenderHistory(report, codex.RenderOptions{
		Color: shouldUseColor(*noColor),
	}))
}

func historyFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("codexstat history", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s history [flags]\n\n", os.Args[0])
		fmt.Fprintln(flags.Output(), "Print day-over-day token usage from Codex session logs.")
		fmt.Fprintln(flags.Output(), "\nFlags:")
		flags.PrintDefaults()
	}
	return flags
}

func shouldUseColor(noColor bool) bool {
	if noColor {
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if forceColorEnabled() {
		return true
	}
	info, err := os.Stdout.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func forceColorEnabled() bool {
	for _, key := range []string{"FORCE_COLOR", "CLICOLOR_FORCE"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

func writeJSON(value any, pretty bool) {
	enc := json.NewEncoder(os.Stdout)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(value); err != nil {
		exitErr(err, 1)
	}
}

func exitErr(err error, code int) {
	fmt.Fprintln(os.Stderr, "codexstat:", err)
	os.Exit(code)
}
