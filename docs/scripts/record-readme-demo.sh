#!/bin/bash
# Record the README terminal demo GIF from the local codexstat output.
#
# Prerequisites:
#   brew install asciinema
#   cargo install --git https://github.com/asciinema/agg agg
#
# This intentionally uses the caller's real Codex auth/session data so the
# README image matches the actual pretty-printed output. The command runs with
# --no-record to avoid appending a quota snapshot while recording.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR="$REPO_ROOT/docs/images"
OUTPUT="$OUTPUT_DIR/codexstat-demo.gif"

WORK_DIR="$(mktemp -d)"
DEMO_BIN_DIR="$WORK_DIR/bin"

cleanup() {
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

mkdir -p "$OUTPUT_DIR" "$DEMO_BIN_DIR"

if ! command -v asciinema >/dev/null 2>&1; then
	echo "Error: asciinema not found. Install with: brew install asciinema" >&2
	exit 1
fi

if ! command -v agg >/dev/null 2>&1; then
	echo "Error: agg not found. Install with: cargo install --git https://github.com/asciinema/agg agg" >&2
	exit 1
fi

(
	cd "$REPO_ROOT"
	go build -o "$DEMO_BIN_DIR/codexstat" ./cmd/codexstat
)

export PATH="$DEMO_BIN_DIR:$PATH"
export CODEXSTAT_HISTORY="$WORK_DIR/history.jsonl"
unset NO_COLOR

CAST_FILE="${OUTPUT%.gif}.cast"
DEMO_SCRIPT="$WORK_DIR/run-demo.sh"

cat > "$DEMO_SCRIPT" <<'SH'
#!/bin/bash
set -euo pipefail
sleep 0.1
printf '\033[?25l\033[2J\033[H'
codexstat --no-record
sleep 2.5
SH
chmod +x "$DEMO_SCRIPT"

echo "Recording codexstat output to $OUTPUT..."
echo "Terminal: 118x40"

asciinema rec "$CAST_FILE" \
	--window-size "118x40" \
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
