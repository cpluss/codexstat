package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"codexstat/internal/codex"
)

const version = "0.1.0"

func main() {
	var (
		sourceFlag = flag.String("source", "auto", "data source: auto, oauth, or cli")
		jsonFlag   = flag.Bool("json", false, "print JSON instead of text")
		prettyFlag = flag.Bool("pretty", false, "pretty-print JSON output")
		noRefresh  = flag.Bool("no-refresh", false, "do not refresh stale OAuth tokens")
		codexHome  = flag.String("codex-home", "", "Codex home directory containing auth.json and config.toml")
		codexBin   = flag.String("codex-bin", "codex", "codex executable path or name for CLI fallback")
		timeout    = flag.Duration("timeout", 15*time.Second, "overall fetch timeout")
		showVer    = flag.Bool("version", false, "print version and exit")
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Print current Codex account, credit, and rate-limit stats.")
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println(version)
		return
	}

	source, err := codex.ParseSource(*sourceFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "codexstat:", err)
		os.Exit(2)
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
		fmt.Fprintln(os.Stderr, "codexstat:", err)
		os.Exit(1)
	}

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		if *prettyFlag {
			enc.SetIndent("", "  ")
		}
		if err := enc.Encode(snapshot); err != nil {
			fmt.Fprintln(os.Stderr, "codexstat:", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println(codex.RenderText(snapshot))
}
