#!/bin/bash
# Capture the README terminal screenshot from the local codexstat output.
#
# Prerequisites:
#   brew install asciinema
#   cargo install --git https://github.com/asciinema/agg agg
#   brew install imagemagick
#
# This intentionally uses the caller's real Codex auth/session data so the
# README image matches the actual pretty-printed output. The command runs with
# --no-record to avoid appending a quota snapshot while recording.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUTPUT_DIR="$REPO_ROOT/docs/images"
OUTPUT="$OUTPUT_DIR/codexstat-demo.png"

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

if ! command -v magick >/dev/null 2>&1; then
	echo "Error: magick not found. Install with: brew install imagemagick" >&2
	exit 1
fi

(
	cd "$REPO_ROOT"
	go build -o "$DEMO_BIN_DIR/codexstat" ./cmd/codexstat
)

export PATH="$DEMO_BIN_DIR:$PATH"
export CODEXSTAT_HISTORY="$WORK_DIR/history.jsonl"
export FORCE_COLOR=1
export CLICOLOR_FORCE=1
export TERM=xterm-256color
export COLORTERM=truecolor
unset NO_COLOR

CAST_FILE="$WORK_DIR/codexstat-demo.cast"
TMP_GIF="$WORK_DIR/codexstat-demo.gif"
TMP_FLAT_GIF="$WORK_DIR/codexstat-demo-flat.gif"
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

echo "Capturing codexstat output to $OUTPUT..."
echo "Terminal: 118x40"

asciinema rec "$CAST_FILE" \
	--window-size "118x40" \
	--overwrite \
	-c "bash '$DEMO_SCRIPT'"

echo "Rendering final frame..."

agg "$CAST_FILE" "$TMP_GIF" \
	--font-family "JetBrains Mono,Menlo,Monaco,monospace" \
	--font-size 11 \
	--theme dracula \
	--speed 1

magick "$TMP_GIF" \
	-coalesce \
	-fill "#1e1e2d" \
	-opaque "#282a36" \
	-background "#1e1e2d" \
	-alpha remove \
	-alpha off \
	"$TMP_FLAT_GIF"

magick "${TMP_FLAT_GIF}[-1]" "$OUTPUT"

echo "Captured $OUTPUT"
