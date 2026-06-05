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

func main() {
	run(os.Args[1:])
}

func run(args []string) {
	jsonFlag, err := parseArgs(args)
	if err != nil {
		exitErr(err, 2)
	}

	timeout := 15 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	snapshot, err := codex.Fetch(ctx, codex.Options{
		Source:        codex.SourceAuto,
		CodexBin:      "codex",
		ClientVersion: versionString(),
		Timeout:       timeout,
	})
	if err != nil {
		exitErr(err, 1)
	}

	path, pathErr := codex.DefaultHistoryPath(nil)
	if pathErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, "history path unavailable: "+pathErr.Error())
	} else if err := codex.RecordSnapshot(path, snapshot); err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "history write failed: "+err.Error())
	}

	progress, finishProgress := tokenUsageProgressPrinter(isTerminal(os.Stderr))
	defer finishProgress()
	now := time.Now()
	tokenUsageQuery := codex.TokenUsageQuery{
		All:      true,
		Metric:   "tokens",
		Now:      now,
		Progress: progress,
	}
	tokenUsage, err := codex.BuildTokenUsageReport(tokenUsageQuery)
	if err != nil {
		finishProgress()
		snapshot.Warnings = append(snapshot.Warnings, "token usage unavailable: "+err.Error())
		tokenUsage = codex.EmptyTokenUsageReport(tokenUsageQuery)
	}
	finishProgress()
	snapshot.TokenUsage = &tokenUsage

	if jsonFlag {
		writeJSON(snapshot)
		return
	}

	fmt.Println(codex.RenderText(snapshot, codex.RenderOptions{
		Color: shouldUseColor(),
		Now:   now,
	}))
}

func parseArgs(args []string) (bool, error) {
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		if args[0] == "--json" {
			return true, nil
		}
	}
	return false, fmt.Errorf("usage: %s [--json]", os.Args[0])
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
