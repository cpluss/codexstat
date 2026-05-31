#!/bin/bash
# Record the README terminal demo GIF.
#
# Prerequisites:
#   brew install asciinema
#   cargo install --git https://github.com/asciinema/agg agg

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR="$REPO_ROOT/docs/images"
OUTPUT="$OUTPUT_DIR/codexstat-demo.gif"

WORK_DIR="$(mktemp -d)"
DEMO_HOME="$WORK_DIR/codex-home"
DEMO_BIN_DIR="$WORK_DIR/bin"
SERVER_PID=""

cleanup() {
	if [[ -n "$SERVER_PID" ]]; then
		kill "$SERVER_PID" >/dev/null 2>&1 || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$OUTPUT_DIR" "$DEMO_HOME" "$DEMO_BIN_DIR"

if ! command -v asciinema >/dev/null 2>&1; then
	echo "Error: asciinema not found. Install with: brew install asciinema" >&2
	exit 1
fi

if ! command -v agg >/dev/null 2>&1; then
	echo "Error: agg not found. Install with: cargo install --git https://github.com/asciinema/agg agg" >&2
	exit 1
fi

python3 - "$DEMO_HOME" <<'PY'
import json
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

home = Path(sys.argv[1])
sessions = home / "sessions"
today = datetime.now(timezone.utc).date()

daily = [
    (6, 420, 80, 64, 6),
    (5, 860, 210, 96, 11),
    (4, 520, 160, 88, 7),
    (3, 1480, 420, 190, 22),
    (2, 740, 140, 115, 9),
    (1, 1120, 310, 160, 18),
    (0, 980, 220, 140, 13),
]

for days_ago, input_tokens, cached_tokens, output_tokens, reasoning_tokens in daily:
    day = today - timedelta(days=days_ago)
    day_dir = sessions / f"{day:%Y}" / f"{day:%m}" / f"{day:%d}"
    day_dir.mkdir(parents=True, exist_ok=True)
    total_tokens = input_tokens + output_tokens
    event = {
        "timestamp": f"{day.isoformat()}T11:30:00Z",
        "type": "event_msg",
        "payload": {
            "type": "token_count",
            "info": {
                "last_token_usage": {
                    "input_tokens": input_tokens,
                    "cached_input_tokens": cached_tokens,
                    "output_tokens": output_tokens,
                    "reasoning_output_tokens": reasoning_tokens,
                    "total_tokens": total_tokens,
                }
            },
        },
    }
    path = day_dir / f"rollout-{day.isoformat()}T11-30-00-demo.jsonl"
    path.write_text(json.dumps(event) + "\n", encoding="utf-8")
PY

cat > "$WORK_DIR/demo_usage_server.py" <<'PY'
import json
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

home = Path(sys.argv[1])


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path != "/api/codex/usage":
            self.send_response(404)
            self.end_headers()
            return

        now = int(time.time())
        payload = {
            "plan_type": "pro",
            "rate_limit": {
                "primary_window": {
                    "used_percent": 34,
                    "reset_at": now + 7800,
                    "limit_window_seconds": 18000,
                },
                "secondary_window": {
                    "used_percent": 58,
                    "reset_at": now + 345600,
                    "limit_window_seconds": 604800,
                },
            },
            "additional_rate_limits": [
                {
                    "limit_name": "GPT-5 Codex Spark",
                    "metered_feature": "spark",
                    "rate_limit": {
                        "primary_window": {
                            "used_percent": 18,
                            "reset_at": now + 7800,
                            "limit_window_seconds": 18000,
                        },
                        "secondary_window": {
                            "used_percent": 41,
                            "reset_at": now + 345600,
                            "limit_window_seconds": 604800,
                        },
                    },
                }
            ],
            "credits": {"has_credits": True, "unlimited": False, "balance": "42.50"},
        }
        body = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
port = server.server_address[1]
(home / "auth.json").write_text('{"OPENAI_API_KEY":"demo-token"}\n', encoding="utf-8")
(home / "config.toml").write_text(
    f'chatgpt_base_url = "http://127.0.0.1:{port}"\n',
    encoding="utf-8",
)
server.serve_forever()
PY

python3 "$WORK_DIR/demo_usage_server.py" "$DEMO_HOME" >"$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

for _ in {1..50}; do
	if [[ -f "$DEMO_HOME/config.toml" ]]; then
		break
	fi
	sleep 0.1
done

if [[ ! -f "$DEMO_HOME/config.toml" ]]; then
	echo "Demo usage server did not start" >&2
	cat "$WORK_DIR/server.log" >&2 || true
	exit 1
fi

(
	cd "$REPO_ROOT"
	go build -o "$DEMO_BIN_DIR/codexstat" ./cmd/codexstat
)

export CODEX_HOME="$DEMO_HOME"
export CODEXSTAT_HISTORY="$WORK_DIR/history.jsonl"
export PATH="$DEMO_BIN_DIR:$PATH"
unset NO_COLOR

CAST_FILE="${OUTPUT%.gif}.cast"
DEMO_SCRIPT="$WORK_DIR/run-demo.sh"

cat > "$DEMO_SCRIPT" <<'SH'
#!/bin/bash
set -euo pipefail
sleep 0.1
printf '\033[?25l\033[2J\033[H'
codexstat --source oauth --no-record
sleep 2.5
SH
chmod +x "$DEMO_SCRIPT"

echo "Recording codexstat output to $OUTPUT..."
echo "Terminal: 118x34"

asciinema rec "$CAST_FILE" \
	--window-size "118x34" \
	--overwrite \
	-c "bash '$DEMO_SCRIPT'"

echo "Converting to GIF..."

agg "$CAST_FILE" "$OUTPUT" \
	--font-family "JetBrains Mono,Menlo,Monaco,monospace" \
	--font-size 11 \
	--theme dracula \
	--speed 1

rm -f "$CAST_FILE"

echo "Recorded $OUTPUT"
