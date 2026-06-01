# codexstat

`codexstat` is a small terminal-first CLI for checking current Codex limits and local token usage history.

It is intentionally narrower than CodexBar: there is no menu bar UI and no browser-dashboard scraping. CodexBar deserves credit for proving out useful Codex usage surfaces; this project is a smaller terminal take that keeps the data path direct and easy to inspect.

<p align="center">
  <img src="docs/images/codexstat-demo.png" width="792" alt="codexstat terminal output showing Codex limits and token usage">
</p>

## Features

- Current Codex session, weekly, and model-specific limits.
- Account, plan, credit, source, and scan metadata in JSON output.
- OAuth usage API reads from `~/.codex/auth.json` or `$CODEX_HOME/auth.json`.
- Automatic OAuth token refresh when the local Codex refresh token is stale.
- Local `codex app-server` JSON-RPC fallback when OAuth is unavailable.
- Local token usage history from Codex session JSONL logs.
- Append-only local quota snapshots for session and weekly percentage graphs.
- Unicode terminal tables, progress bars, and token/day charts.

## Requirements

- A working Codex CLI login for live limit data.
- `curl` and `tar` for the shell installer.
- Go 1.26 or newer only when installing from source or when release binaries are unavailable.
- An install directory on `PATH`. The shell installer defaults to `~/.local/bin`; add it if your shell cannot find `codexstat` after install:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## Install

After the GitHub repository is public, install or update the latest release with:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
```

The installer downloads the latest GitHub release for macOS or Linux and installs `codexstat` to `~/.local/bin` by default. Use `CODEXSTAT_INSTALL_DIR` to choose a different directory:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_INSTALL_DIR="$HOME/bin" sh
```

Install a specific release:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_VERSION=v0.4.3 sh
```

If no release binary is available, the installer falls back to `go install` and writes the binary to the same install directory.

Go install remains available as the plain Go path:

```sh
GOBIN="$HOME/.local/bin" go install github.com/cpluss/codexstat/cmd/codexstat@latest
```

From a local checkout, install the current working tree with:

```sh
./install.sh --local
```

Check the installed binary:

```sh
codexstat --version
codexstat
```

## Update

Re-run the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
```

For Go installs:

```sh
GOBIN="$HOME/.local/bin" go install github.com/cpluss/codexstat/cmd/codexstat@latest
```

To track unreleased `main` instead of tagged releases:

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_VERSION=main sh
```

If you installed from a local checkout:

```sh
git pull --ff-only && ./install.sh --local
```

## Quick Start

```sh
codexstat
codexstat --source oauth
codexstat --source cli
codexstat --json --pretty
codexstat tokens --days 30
codexstat history --days 14
codexstat history --metric session
codexstat history --metric weekly
```

The default source is `auto`: `codexstat` tries OAuth first, then falls back to the local Codex CLI app server.

## Commands

### `codexstat`

Print current Codex account, credit, rate-limit, and recent token usage stats.

Useful flags:

| Flag | Description |
| --- | --- |
| `--source auto|oauth|cli` | Choose the live stats source. Default: `auto`. |
| `--json` | Print machine-readable JSON instead of terminal tables. |
| `--pretty` | Pretty-print JSON output. |
| `--codex-home PATH` | Use a Codex home other than `~/.codex`. |
| `--codex-bin PATH` | Use a specific Codex executable for CLI fallback. |
| `--timeout DURATION` | Set the overall live stats fetch timeout. Default: `15s`. |
| `--no-refresh` | Do not refresh stale OAuth tokens. |
| `--no-record` | Do not append this live fetch to quota history. |
| `--history-file PATH` | Store quota snapshots somewhere other than the default path. |
| `--no-color` | Disable ANSI color. `NO_COLOR` is also respected. |
| `--version` | Print the installed version and exit. |

### `codexstat history`

Print a graph from local history. Token metrics are read from Codex session logs. Session and weekly limit metrics are read from `codexstat` quota snapshots.

```sh
codexstat history --days 14
codexstat history --metric input
codexstat history --metric cached
codexstat history --metric output
codexstat history --metric reasoning
codexstat history --metric session
codexstat history --metric weekly
```

History flags:

| Flag | Description |
| --- | --- |
| `--days N` | Number of days to show. Default: `7`. |
| `--metric tokens|input|cached|output|reasoning|session|weekly` | Select the graph metric. Default: `tokens`. |
| `--codex-home PATH` | Use a Codex home other than `~/.codex` for token logs. |
| `--history-file PATH` | Read quota snapshots from a custom path. |
| `--json` | Print machine-readable JSON. |
| `--pretty` | Pretty-print JSON output. |
| `--no-color` | Disable ANSI color. |

### `codexstat tokens`

Shortcut for token history:

```sh
codexstat tokens --days 30
```

This is equivalent to:

```sh
codexstat history --metric tokens --days 30
```

## Data Sources

`codexstat --source oauth` reads the local Codex OAuth credentials from:

- `~/.codex/auth.json`
- `$CODEX_HOME/auth.json`
- the path implied by `--codex-home`

It calls the Codex usage endpoint with the local access token. If the credential file contains a stale refresh token and `--no-refresh` is not set, `codexstat` refreshes the token and writes the updated credentials back to the same Codex auth file.

`codexstat --source cli` starts:

```sh
codex -s read-only -a untrusted app-server
```

Then it reads account and rate-limit data over the local JSON-RPC app-server protocol.

`codexstat --source auto` tries OAuth first and falls back to the CLI source if OAuth fails before the overall timeout.

## Token History

Token usage is scanned from local Codex session logs:

- `~/.codex/sessions/YYYY/MM/DD/*.jsonl`
- `~/.codex/archived_sessions/*.jsonl`

Use `--metric tokens`, `input`, `cached`, `output`, or `reasoning` to choose the graph column.

Every successful live stats fetch also appends a quota snapshot unless `--no-record` is passed. Those recorded samples are used for:

- `codexstat history --metric weekly`
- `codexstat history --metric session`

Default quota snapshot paths:

- macOS: `~/Library/Application Support/codexstat/history.jsonl`
- Linux and other Unix: `$XDG_STATE_HOME/codexstat/history.jsonl` or `~/.local/state/codexstat/history.jsonl`

Set `CODEXSTAT_HISTORY` or pass `--history-file` to use a different file.

## JSON Output

Use JSON for scripts, dashboards, or snapshot diffs:

```sh
codexstat --json --pretty
codexstat history --metric tokens --json --pretty
codexstat history --metric weekly --json --pretty
```

The main JSON output includes the fetched live data and the local token usage report under `token_usage`.

## Privacy

`codexstat` is a local CLI. It reads local Codex auth, config, and session files. For live OAuth stats, it sends requests to the Codex/ChatGPT usage endpoint and to `auth.openai.com` only when a token refresh is needed. For CLI stats, it talks to a local `codex app-server` process.

Local writes are limited to:

- quota snapshots in the `codexstat` history JSONL file;
- refreshed Codex OAuth credentials, only when refresh is needed and `--no-refresh` is not set.

Use `--no-record` to disable quota snapshot writes for a run.

## Development

```sh
go test ./...
go build -o ./codexstat ./cmd/codexstat
./install.sh --local
```

`codexstat --version` is derived from Go build metadata. Tagged public installs report the selected tag; local checkout and branch installs may report a pseudo-version or `dev` with VCS details.

## Release Notes For Maintainers

The public Go install fallback depends on the module path in `go.mod`:

```txt
github.com/cpluss/codexstat
```

Use semantic-version tags so the shell installer and `go install ...@latest` have a stable update path:

```sh
go test ./...
git tag v0.4.3
git push origin v0.4.3
```

Pushing a `v*` tag runs `.github/workflows/release.yml`, builds macOS and Linux tarballs, publishes `checksums.txt`, and creates a GitHub release. After the release is published, verify the installer from outside the checkout:

```sh
tmp="$(mktemp -d)"
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_INSTALL_DIR="$tmp" sh
"$tmp/codexstat" --version
```

Before making the repository broadly public, add the license you want users to follow. Without a license file, the code is visible source but not meaningfully open source for reuse.

## Troubleshooting

`codexstat: Codex auth.json not found`

Run `codex` once and log in, or pass `--source cli` if you want to force the local CLI app-server path.

`codexstat: Codex CLI not found on PATH`

Install the Codex CLI or pass `--codex-bin /path/to/codex`.

`codexstat` installed but the shell cannot find it

Add the install directory to `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

No token history appears

Run Codex normally first so local session JSONL logs exist, then run `codexstat tokens`.

## License

No license file has been committed yet. Add one before treating the public repository as open source.
