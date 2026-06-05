# codexstat

Terminal stats for Codex usage, limits, and local token history.

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
```

`codexstat` is a small local CLI that shows your current Codex session and weekly limits plus all token usage it can find in local Codex session logs. It is built for people who want one terminal command without a menu bar app, browser dashboard scraping, or a pile of switches.

<p align="center">
  <img src="docs/images/codexstat-demo.png" width="792" alt="codexstat terminal output showing Codex limits and token usage">
</p>

## What It Shows

- Current Codex session, weekly, and model-specific limits.
- Token usage summaries, monthly trends, and recent daily activity from local Codex JSONL session logs.
- Automatic quota snapshots for future local history.
- Text tables and charts for humans; JSON output for scripts.

## Install

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
```

The installer downloads a prebuilt release for macOS or Linux and installs `codexstat` to `~/.local/bin`. If no matching release binary is available, it falls back to `go install`.

For a private repository, pass a token that can read releases:

```sh
token="$(gh auth token)"
curl -H "Authorization: Bearer $token" -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | GH_TOKEN="$token" sh
```

Prefer to inspect the installer first?

```sh
curl -fsSLO https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh
less install.sh
sh install.sh
```

If `codexstat` is not found after install, add the default install directory to your shell:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Install to another directory:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_INSTALL_DIR="$HOME/bin" sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_VERSION=v0.4.3 sh
```

Install the latest unreleased build from `main`:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_VERSION=main sh
```

Install from source with Go:

```sh
GOBIN="$HOME/.local/bin" go install github.com/cpluss/codexstat/cmd/codexstat@latest
```

Install the current checkout:

```sh
./install.sh --local
```

## Update

Use the built-in updater:

```sh
codexstat update
```

For a private repository, set `CODEXSTAT_GITHUB_TOKEN`, `GH_TOKEN`, or `GITHUB_TOKEN` so the updater can read the GitHub release API.

Update to a specific version:

```sh
codexstat update v0.4.3
```

Update to the latest unreleased `main` build:

```sh
codexstat update main
```

Or re-run the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
```

If you installed with Go:

```sh
GOBIN="$HOME/.local/bin" go install github.com/cpluss/codexstat/cmd/codexstat@latest
```

If you installed from a checkout:

```sh
git pull --ff-only && ./install.sh --local
```

## Requirements

- macOS or Linux.
- A working Codex CLI login for live limit data.
- `curl` and `tar` for the shell installer.
- Go 1.26 or newer only for source installs or installer fallback builds.

## Quick Start

```sh
codexstat
codexstat --json
codexstat --version
```

`codexstat` prints a summarized terminal view; `codexstat --json` prints machine-readable JSON with the full underlying data; `codexstat update` replaces the installed binary with a downloaded GitHub release asset after verifying `checksums.txt`.

By default, `codexstat` tries Codex OAuth usage data first, falls back to a local `codex app-server` process when needed, records a quota snapshot, and scans all local token history. The terminal view is intentionally summarized; use JSON when you need every daily row.

## Usage

Print the terminal report:

```sh
codexstat
```

Print compact JSON:

```sh
codexstat --json
```

`NO_COLOR` disables ANSI color in text output. `FORCE_COLOR` and `CLICOLOR_FORCE` force color when set to a non-zero value.

## Data Sources

`codexstat` can read live stats in two ways.

The OAuth source reads local Codex credentials from:

- `~/.codex/auth.json`
- `$CODEX_HOME/auth.json`

It calls the Codex usage endpoint with the local access token. If the credential file contains a stale refresh token, `codexstat` refreshes it and writes the updated credentials back to the same Codex auth file.

If OAuth is unavailable, `codexstat` starts:

```sh
codex -s read-only -a untrusted app-server
```

Then it reads account and rate-limit data over the local JSON-RPC app-server protocol.

Live quota snapshots are cached for quick repeat launches. If the cached live stats are less than five minutes old, `codexstat` uses them directly and skips the OAuth/API fetch. If live refresh fails later, `codexstat` falls back to the cached snapshot with a warning.

Default live snapshot cache paths:

- macOS: `~/Library/Caches/codexstat/snapshot.json`
- Linux and other Unix: `$XDG_CACHE_HOME/codexstat/snapshot.json` or `~/.cache/codexstat/snapshot.json`

Set `CODEXSTAT_SNAPSHOT_CACHE` to use a different live snapshot cache file.

## Local History

Token usage is scanned from every local Codex session log `codexstat` can find:

- `~/.codex/sessions/YYYY/MM/DD/*.jsonl`
- `~/.codex/archived_sessions/*.jsonl`

The first run builds a local SQLite cache containing only the file metadata and per-day token aggregates needed for reports. Later runs refresh that cache incrementally by checking session log size and modification time, then reparsing only new or changed files from the latest cached date minus one day through today. Interactive refreshes print a compact progress line on stderr. Normal text and JSON results stay on stdout.

Default token usage cache paths:

- macOS: `~/Library/Caches/codexstat/token_usage.sqlite`
- Linux and other Unix: `$XDG_CACHE_HOME/codexstat/token_usage.sqlite` or `~/.cache/codexstat/token_usage.sqlite`

Set `CODEXSTAT_TOKEN_USAGE_CACHE` to use a different token usage cache file.

The terminal report summarizes that local history into a small dashboard:

- current usage periods such as today, last 7 days, last 30 days, this month, last month, and all local history;
- monthly usage with active-day counts and each month's peak day;
- a last-30-days chart and a short recent-days table.

Quota snapshots are recorded after successful live stats fetches.

Default quota snapshot paths:

- macOS: `~/Library/Application Support/codexstat/history.jsonl`
- Linux and other Unix: `$XDG_STATE_HOME/codexstat/history.jsonl` or `~/.local/state/codexstat/history.jsonl`

Set `CODEXSTAT_HISTORY` to use a different quota snapshot file.

## JSON Output

Use JSON for scripts, dashboards, or snapshot diffs:

```sh
codexstat --json
```

The JSON output includes fetched live data and the full local token usage report under `token_usage`, including the complete daily token history that the terminal view summarizes.

## Privacy

`codexstat` runs locally and does not run a background service.

It reads:

- local Codex auth and config files;
- local Codex session JSONL logs;
- the local `codex app-server` when using the CLI source.

It writes:

- live quota snapshots to the `codexstat` snapshot cache file;
- quota snapshots to the `codexstat` history JSONL file;
- token usage aggregates to the `codexstat` SQLite cache file;
- refreshed Codex OAuth credentials, only when refresh is needed.

For live OAuth stats, it sends requests to the Codex/ChatGPT usage endpoint and to `auth.openai.com` only when a token refresh is needed.

## Development

Clone the repo and run the tests:

```sh
git clone https://github.com/cpluss/codexstat.git
cd codexstat
go test ./...
```

Build and run locally:

```sh
go build -o ./codexstat ./cmd/codexstat
./codexstat
```

Install the checkout:

```sh
./install.sh --local
```

## Contributing

Issues and pull requests are welcome.

Before opening a PR:

- run `go test ./...`;
- keep changes focused;
- update the README when behavior, flags, install paths, or data sources change;
- avoid committing real Codex auth files, session logs, or private usage output.

If you change terminal rendering, regenerate or update the demo image only when the visible output actually changes.

## Maintainer Notes

The release workflow publishes two channels:

- pushing to `main` updates the moving `nightly` prerelease;
- pushing a `v*` tag publishes a stable GitHub release.

To publish a stable release:

```sh
go test ./...
git tag v0.4.3
git push origin v0.4.3
```

`.github/workflows/release.yml` builds macOS and Linux tarballs, publishes `checksums.txt`, and creates or updates the GitHub release.

After a stable release is published, verify the installer from outside the checkout:

```sh
tmp="$(mktemp -d)"
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_INSTALL_DIR="$tmp" sh
"$tmp/codexstat" --json
```

The public Go install fallback depends on the module path in `go.mod`:

```txt
github.com/cpluss/codexstat
```

## Troubleshooting

`codexstat: Codex auth.json not found`

Run `codex` once and log in.

`codexstat: Codex CLI not found on PATH`

Install the Codex CLI and make sure `codex` is on `PATH`.

`codexstat` installed but the shell cannot find it

Add the install directory to `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

No token history appears

Run Codex normally first so local session JSONL logs exist, then run `codexstat`.

## License

No license file has been committed yet. Add one before treating this repository as reusable open-source software.
