# codexstat

Agent-native usage introspection for Codex.

```sh
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
codexstat
```

`codexstat` shows how much Codex capacity you have right now, how much you have used over time, and the same data as JSON for agents and scripts.

<p align="center">
  <img src="docs/images/codexstat-demo.png" width="792" alt="codexstat terminal output showing Codex limits and token usage">
</p>

## Highlights

- Current Codex usage windows, remaining capacity, and reset timing.
- Token history across days and months.
- Quick summaries for today, recent days, this month, last month, and all local history.
- A compact terminal report for humans.
- Structured JSON for agents, scripts, and dashboards.
- No daemon, no menu bar app, and no browser dashboard scraping.

## Why Use It

Codex usage should be state an agent can inspect, not a dashboard a human has to open.

With `codexstat --json`, an agent can check usage before starting work, compare recent activity with longer-term history, decide when to slow down, and leave an auditable snapshot behind.

## Install

The installer above installs the latest macOS or Linux release to `~/.local/bin`. If your shell cannot find `codexstat` afterwards:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Prefer Go?

```sh
GOBIN="$HOME/.local/bin" go install github.com/cpluss/codexstat/cmd/codexstat@latest
```

Need a pinned version, custom install directory, private release token, or local checkout install?

See `install.sh --help`.

## Quick Examples

```sh
codexstat
codexstat --json
```

```sh
codexstat --json | jq '.token_usage.total'
codexstat --json | jq '.token_usage.days[-7:]'
```

## Commands

| Command | Description |
| --- | --- |
| `codexstat` | Print the terminal usage report. |
| `codexstat --json` | Print the full machine-readable snapshot. |
| `codexstat --version` | Print the installed version. |
| `codexstat update [VERSION]` | Update the installed binary. |

## Requirements

- macOS or Linux.
- A working Codex CLI login for live usage data.
- Local Codex session history for long-term token reporting.
- `curl` and `tar` for release installs.
- Go 1.26 or newer for source installs.

## Output

The terminal report is for humans: current limits first, then token history summaries and charts.

The JSON report is for agents and automation. It includes the live usage snapshot, warnings, token totals, and daily token history.

JSON output is safe to pipe into tools such as `jq`.

## Privacy

`codexstat` runs locally. There is no hosted dashboard, background service, or account to create.

Live usage checks contact the OpenAI/Codex services needed to report current account limits. Token history comes from your local Codex history.

## Troubleshooting

`codexstat: Codex auth.json not found`

Run `codex` once and log in.

`codexstat: Codex CLI not found on PATH`

Install the Codex CLI and make sure `codex` is on `PATH`.

No token history appears

Run Codex normally first so local session history exists, then run `codexstat`.

## Development

```sh
git clone https://github.com/cpluss/codexstat.git
cd codexstat
go test ./...
go build -o ./codexstat ./cmd/codexstat
./codexstat
```

Install the current checkout:

```sh
./install.sh --local
```

Before opening a PR:

- run `go test ./...`;
- keep changes focused;
- update docs when behaviour, flags, install paths, or data sources change;
- avoid committing real Codex auth files, session logs, or private usage output.

Release notes for maintainers live in [docs/RELEASING.md](docs/RELEASING.md).

## License

MIT. See [LICENSE](LICENSE).
