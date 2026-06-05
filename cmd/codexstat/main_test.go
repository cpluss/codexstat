package main

import "testing"

func TestParseArgsOnlyAllowsJSONFlag(t *testing.T) {
	json, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if json {
		t.Fatal("no args should use text output")
	}

	json, err = parseArgs([]string{"--json"})
	if err != nil {
		t.Fatal(err)
	}
	if !json {
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
