package main

import "testing"

func TestParseArgsAllowsReportMode(t *testing.T) {
	opts, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != commandReport {
		t.Fatalf("no args command = %v, want report", opts.command)
	}
	if opts.json {
		t.Fatal("no args should use text output")
	}

	opts, err = parseArgs([]string{"--json"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != commandReport {
		t.Fatalf("--json command = %v, want report", opts.command)
	}
	if !opts.json {
		t.Fatal("--json should enable JSON output")
	}

	for _, args := range [][]string{
		{"--pretty"},
		{"tokens"},
		{"history"},
		{"--json", "--pretty"},
	} {
		if _, err := parseArgs(args); err == nil {
			t.Fatalf("parseArgs(%v) succeeded, want error", args)
		}
	}
}

func TestParseArgsAllowsVersionAndUpdate(t *testing.T) {
	opts, err := parseArgs([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != commandVersion {
		t.Fatalf("--version command = %v, want version", opts.command)
	}

	opts, err = parseArgs([]string{"update"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != commandUpdate {
		t.Fatalf("update command = %v, want update", opts.command)
	}
	if opts.updateVersion != "" {
		t.Fatalf("updateVersion = %q, want empty", opts.updateVersion)
	}

	opts, err = parseArgs([]string{"update", "nightly"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != commandUpdate || opts.updateVersion != "nightly" {
		t.Fatalf("parseArgs(update nightly) = %+v", opts)
	}

	opts, err = parseArgs([]string{"update", "--version", "v1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.command != commandUpdate || opts.updateVersion != "v1.2.3" {
		t.Fatalf("parseArgs(update --version v1.2.3) = %+v", opts)
	}
}
