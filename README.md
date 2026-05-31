# codexstat

`codexstat` is a small Go CLI for printing current Codex stats and day-over-day token usage.

It is intentionally narrower than CodexBar: it has no menu bar UI and no browser-dashboard scraping. The first implementation mirrors the low-friction CodexBar data paths:

- OAuth usage API from `~/.codex/auth.json` or `$CODEX_HOME/auth.json`
- automatic OAuth token refresh after 8 days
- `codex app-server` JSON-RPC fallback for local CLI usage
- Codex session, weekly, additional model-specific limits, account, plan, and credits
- Codex token usage scanned from local session JSONL logs
- append-only local JSONL quota snapshot history for session/weekly percentage graphs
- Unicode terminal tables, progress bars, and token/day charts via Charmbracelet-style rendering libraries

## Build

```sh
go build ./cmd/codexstat
```

## Use

```sh
./codexstat
./codexstat --source oauth
./codexstat --source cli
./codexstat --json --pretty
./codexstat --codex-home ~/.codex
./codexstat history --days 14
./codexstat history --metric input
./codexstat tokens --days 30
./codexstat history --metric session  # quota snapshot history
```

The default source is `auto`: OAuth first, then local `codex app-server` if OAuth is unavailable.

The main `codexstat` output always includes a 7-day token usage table. The same data is also included in `--json` output under `token_usage`.

`codexstat history` is the focused token-history view and defaults to token totals. It scans:

- `~/.codex/sessions/YYYY/MM/DD/*.jsonl`
- `~/.codex/archived_sessions/*.jsonl`

Use `--metric tokens`, `input`, `cached`, `output`, or `reasoning` to choose the graph column.

Every successful fetch also appends a quota snapshot unless `--no-record` is passed. Those recorded samples are used only for `codexstat history --metric weekly` and `codexstat history --metric session`. The default quota history path is:

- macOS: `~/Library/Application Support/codexstat/history.jsonl`
- Linux/other Unix: `$XDG_STATE_HOME/codexstat/history.jsonl` or `~/.local/state/codexstat/history.jsonl`

Set `CODEXSTAT_HISTORY` or pass `--history-file` to use a different file.

## Notes

- `CODEX_HOME` is respected when set.
- `~/.codex/config.toml` is checked for `chatgpt_base_url`.
- Browser-only CodexBar extras such as dashboard usage breakdowns and code-review remaining are not included yet.
