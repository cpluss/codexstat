#!/bin/sh
set -eu

REPO="${CODEXSTAT_REPO:-cpluss/codexstat}"
BIN="codexstat"
VERSION="${CODEXSTAT_VERSION:-latest}"
INSTALL_DIR="${CODEXSTAT_INSTALL_DIR:-}"
METHOD="${CODEXSTAT_INSTALL_METHOD:-auto}"
LOCAL_BUILD=0

usage() {
	cat <<EOF
Install codexstat.

Usage:
  install.sh [options]

Options:
  --dir PATH            Install directory. Default: \$CODEXSTAT_INSTALL_DIR or ~/.local/bin.
  --version VERSION     Version to install. Default: \$CODEXSTAT_VERSION or latest.
  --method METHOD       auto, release, or go. Default: \$CODEXSTAT_INSTALL_METHOD or auto.
  --local               Build the current checkout instead of downloading a release.
  -h, --help            Show this help.

Examples:
  curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_VERSION=v0.4.3 sh
  curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_VERSION=main sh
  CODEXSTAT_INSTALL_DIR="$HOME/bin" ./install.sh
  ./install.sh --local
EOF
}

log() {
	printf 'codexstat: %s\n' "$*" >&2
}

die() {
	log "error: $*"
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--dir)
			[ "$#" -ge 2 ] || die "--dir requires a value"
			INSTALL_DIR="$2"
			shift 2
			;;
		--version)
			[ "$#" -ge 2 ] || die "--version requires a value"
			VERSION="$2"
			shift 2
			;;
		--method)
			[ "$#" -ge 2 ] || die "--method requires a value"
			METHOD="$2"
			shift 2
			;;
		--local)
			LOCAL_BUILD=1
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown option: $1"
			;;
	esac
done

case "$METHOD" in
	auto | release | go) ;;
	*) die "--method must be auto, release, or go" ;;
esac

if [ -z "$INSTALL_DIR" ]; then
	if [ -n "${GOBIN:-}" ]; then
		INSTALL_DIR="$GOBIN"
	else
		[ -n "${HOME:-}" ] || die "HOME is not set; pass --dir or CODEXSTAT_INSTALL_DIR"
		INSTALL_DIR="$HOME/.local/bin"
	fi
fi

case "$INSTALL_DIR" in
	~) INSTALL_DIR="$HOME" ;;
	~/*) INSTALL_DIR="$HOME/${INSTALL_DIR#~/}" ;;
esac

mkdir -p "$INSTALL_DIR"

TMPDIR_ROOT="${TMPDIR:-/tmp}"
TMP="$TMPDIR_ROOT/codexstat-install.$$"
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

detect_target() {
	case "$(uname -s)" in
		Darwin) OS="darwin" ;;
		Linux) OS="linux" ;;
		*) return 1 ;;
	esac

	case "$(uname -m)" in
		x86_64 | amd64) ARCH="amd64" ;;
		arm64 | aarch64) ARCH="arm64" ;;
		*) return 1 ;;
	esac

	return 0
}

latest_tag() {
	need_cmd curl
	curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		return 1
	fi
}

verify_checksum() {
	asset="$1"
	checksums="$TMP/checksums.txt"
	line="$(grep "  $asset\$" "$checksums" 2>/dev/null || grep " $asset\$" "$checksums" 2>/dev/null || true)"
	[ -n "$line" ] || die "checksum for $asset not found in checksums.txt"

	expected="$(printf '%s\n' "$line" | awk '{print $1}')"
	actual="$(sha256_file "$TMP/$asset" || true)"
	if [ -z "$actual" ]; then
		log "warning: sha256sum/shasum not found; skipping checksum verification"
		return 0
	fi

	[ "$expected" = "$actual" ] || die "checksum mismatch for $asset"
}

install_binary() {
	src="$1"
	dst="$INSTALL_DIR/$BIN"
	cp "$src" "$dst"
	chmod 0755 "$dst"
}

install_from_release() {
	detect_target || return 1
	command -v curl >/dev/null 2>&1 || return 1
	command -v tar >/dev/null 2>&1 || return 1

	tag="$VERSION"
	if [ "$tag" = "latest" ]; then
		tag="$(latest_tag || true)"
		[ -n "$tag" ] || return 1
	elif [ "$tag" = "main" ]; then
		tag="nightly"
	fi

	asset="${BIN}_${OS}_${ARCH}.tar.gz"
	base_url="https://github.com/$REPO/releases/download/$tag"

	log "downloading $REPO $tag for $OS/$ARCH"
	if ! curl -fL "$base_url/$asset" -o "$TMP/$asset"; then
		return 1
	fi

	if curl -fL "$base_url/checksums.txt" -o "$TMP/checksums.txt"; then
		verify_checksum "$asset"
	else
		log "warning: checksums.txt unavailable; skipping checksum verification"
	fi

	tar -xzf "$TMP/$asset" -C "$TMP"
	[ -x "$TMP/$BIN" ] || die "$asset did not contain an executable $BIN"
	install_binary "$TMP/$BIN"
	log "installed $BIN to $INSTALL_DIR/$BIN"
}

install_from_go() {
	need_cmd go
	target="$VERSION"
	if [ "$target" = "latest" ]; then
		target="latest"
	fi
	log "building $REPO@$target with go install"
	GOBIN="$INSTALL_DIR" go install "github.com/$REPO/cmd/$BIN@$target"
	log "installed $BIN to $INSTALL_DIR/$BIN"
}

install_from_local_checkout() {
	need_cmd go
	version="${CODEXSTAT_VERSION:-}"
	if [ -z "$version" ] && command -v git >/dev/null 2>&1; then
		version="$(git describe --tags --always --dirty 2>/dev/null || true)"
	fi
	[ -n "$version" ] || version="dev"

	log "building current checkout"
	go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$INSTALL_DIR/$BIN" ./cmd/codexstat
	chmod 0755 "$INSTALL_DIR/$BIN"
	log "installed $BIN to $INSTALL_DIR/$BIN"
}

if [ "$LOCAL_BUILD" -eq 1 ]; then
	install_from_local_checkout
elif [ "$METHOD" = "release" ]; then
	install_from_release || die "release install failed"
elif [ "$METHOD" = "go" ]; then
	install_from_go
else
	if ! install_from_release; then
		log "release binary unavailable; falling back to go install"
		install_from_go
	fi
fi

case ":${PATH:-}:" in
	*":$INSTALL_DIR:"*) ;;
	*) log "add $INSTALL_DIR to PATH if your shell cannot find $BIN" ;;
esac
