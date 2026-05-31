# codexstat

`codexstat` is a small Go CLI for printing current Codex usage stats on demand.

It is intentionally narrower than CodexBar: it has no menu bar UI and no browser-dashboard scraping. The first implementation mirrors the low-friction CodexBar data paths:

- OAuth usage API from `~/.codex/auth.json` or `$CODEX_HOME/auth.json`
- automatic OAuth token refresh after 8 days
- `codex app-server` JSON-RPC fallback for local CLI usage
- Codex session, weekly, additional model-specific limits, account, plan, and credits

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
```

The default source is `auto`: OAuth first, then local `codex app-server` if OAuth is unavailable.

## Notes

- `CODEX_HOME` is respected when set.
- `~/.codex/config.toml` is checked for `chatgpt_base_url`.
- Browser-only CodexBar extras such as dashboard usage breakdowns and code-review remaining are not included yet.
