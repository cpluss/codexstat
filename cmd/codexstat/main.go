package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cpluss/codexstat/internal/codex"
)

var version = "dev"

const liveSnapshotCacheMaxAge = 5 * time.Minute

type command int

const (
	commandReport command = iota
	commandUpdate
	commandVersion
)

type cliOptions struct {
	command       command
	json          bool
	updateVersion string
}

func main() {
	run(os.Args[1:])
}

func run(args []string) {
	opts, err := parseArgs(args)
	if err != nil {
		exitErr(err, 2)
	}

	switch opts.command {
	case commandUpdate:
		if err := selfUpdate(context.Background(), updateOptions{
			Version: opts.updateVersion,
			Stdout:  os.Stdout,
		}); err != nil {
			exitErr(err, 1)
		}
		return
	case commandVersion:
		fmt.Println(versionString())
		return
	}

	updateNoticeDone := startUpdateNoticeCheck(!opts.json)

	timeout := 15 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	progress, finishProgress := tokenUsageProgressPrinter(isTerminal(os.Stderr))
	defer finishProgress()
	now := time.Now()
	tokenUsageQuery := codex.TokenUsageQuery{
		All:      true,
		Metric:   "tokens",
		Now:      now,
		Progress: progress,
	}
	type tokenUsageResult struct {
		report codex.TokenUsageReport
		err    error
	}
	tokenUsageDone := make(chan tokenUsageResult, 1)
	go func() {
		report, err := codex.BuildTokenUsageReport(tokenUsageQuery)
		tokenUsageDone <- tokenUsageResult{report: report, err: err}
	}()

	snapshot, fetchedLive, err := loadOrFetchSnapshot(ctx, timeout)
	if err != nil {
		finishProgress()
		exitErr(err, 1)
	}

	if fetchedLive {
		path, pathErr := codex.DefaultHistoryPath(nil)
		if pathErr != nil {
			snapshot.Warnings = append(snapshot.Warnings, "history path unavailable: "+pathErr.Error())
		} else if err := codex.RecordSnapshot(path, snapshot); err != nil {
			snapshot.Warnings = append(snapshot.Warnings, "history write failed: "+err.Error())
		}
	}

	result := <-tokenUsageDone
	var tokenUsage codex.TokenUsageReport
	if result.err != nil {
		finishProgress()
		snapshot.Warnings = append(snapshot.Warnings, "token usage unavailable: "+result.err.Error())
		tokenUsage = codex.EmptyTokenUsageReport(tokenUsageQuery)
	} else {
		tokenUsage = result.report
	}
	finishProgress()
	snapshot.TokenUsage = &tokenUsage

	if opts.json {
		writeJSON(snapshot)
		return
	}

	useColor := shouldUseColor()
	if notice, ok := receiveUpdateNotice(updateNoticeDone); ok {
		fmt.Println(renderUpdateNotice(notice, useColor))
		fmt.Println()
	}

	fmt.Println(codex.RenderText(snapshot, codex.RenderOptions{
		Color: useColor,
		Now:   now,
	}))
}

func loadOrFetchSnapshot(ctx context.Context, timeout time.Duration) (*codex.Snapshot, bool, error) {
	now := time.Now()
	cachePath, cachePathErr := codex.DefaultSnapshotCachePath(nil)
	if cachePathErr == nil {
		if cached, fresh, err := codex.FreshSnapshotFromCache(cachePath, now, liveSnapshotCacheMaxAge); err == nil && fresh {
			return cached, false, nil
		}
	}

	snapshot, err := codex.Fetch(ctx, codex.Options{
		Source:        codex.SourceAuto,
		CodexBin:      "codex",
		ClientVersion: versionString(),
		Timeout:       timeout,
	})
	if err != nil {
		if cachePathErr == nil {
			if record, loadErr := codex.LoadSnapshotCache(cachePath, now); loadErr == nil && record != nil {
				record.Snapshot.Warnings = append(record.Snapshot.Warnings, "live stats unavailable; using cached snapshot from "+record.CachedAt.Local().Format(time.RFC3339))
				return &record.Snapshot, false, nil
			}
		}
		return nil, false, err
	}

	if cachePathErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, "snapshot cache path unavailable: "+cachePathErr.Error())
	} else if err := codex.RecordSnapshotCache(cachePath, snapshot, time.Now()); err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "snapshot cache write failed: "+err.Error())
	}
	return snapshot, true, nil
}

func parseArgs(args []string) (cliOptions, error) {
	switch len(args) {
	case 0:
		return cliOptions{command: commandReport}, nil
	case 1:
		if args[0] == "--json" {
			return cliOptions{command: commandReport, json: true}, nil
		}
		if args[0] == "--version" || args[0] == "version" {
			return cliOptions{command: commandVersion}, nil
		}
		if args[0] == "update" {
			return cliOptions{command: commandUpdate}, nil
		}
	case 2:
		if args[0] == "update" && args[1] != "" && !strings.HasPrefix(args[1], "-") {
			return cliOptions{command: commandUpdate, updateVersion: args[1]}, nil
		}
	case 3:
		if args[0] == "update" && args[1] == "--version" && args[2] != "" {
			return cliOptions{command: commandUpdate, updateVersion: args[2]}, nil
		}
	}
	return cliOptions{}, fmt.Errorf("usage: %s [--json|--version|update [VERSION]]", os.Args[0])
}

func tokenUsageProgressPrinter(enabled bool) (func(codex.TokenUsageProgress), func()) {
	if !enabled {
		return nil, func() {}
	}

	lastLen := 0
	started := false
	finish := func() {
		if !started {
			return
		}
		fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", lastLen))
		started = false
		lastLen = 0
	}
	progress := func(update codex.TokenUsageProgress) {
		if update.Done {
			finish()
			return
		}

		message := "discovering Codex log files..."
		if update.Phase == "scanning" {
			message = fmt.Sprintf("scanning Codex logs: %d/%d files", update.FilesVisited, update.FilesTotal)
			if update.EventsScanned > 0 {
				message += fmt.Sprintf(", %d token events", update.EventsScanned)
			}
		}
		if len(message) < lastLen {
			message += strings.Repeat(" ", lastLen-len(message))
		}
		fmt.Fprintf(os.Stderr, "\r%s", message)
		lastLen = len(message)
		started = true
	}
	return progress, finish
}

func shouldUseColor() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if forceColorEnabled() {
		return true
	}
	return isTerminal(os.Stdout)
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
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

func versionString() string {
	if trimmed := strings.TrimSpace(version); trimmed != "" && trimmed != "dev" {
		return trimmed
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		return "dev (" + revision + "+modified)"
	}
	return "dev (" + revision + ")"
}

func writeJSON(value any) {
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(value); err != nil {
		exitErr(err, 1)
	}
}

func exitErr(err error, code int) {
	fmt.Fprintln(os.Stderr, "codexstat:", err)
	os.Exit(code)
}
