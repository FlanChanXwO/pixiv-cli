#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-staticlib-script-test.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
mkdir -p "$temporary/bin"

printf '%s\n' \
	'#!/bin/sh' \
	'set -eu' \
	'target=' \
	'target_dir=' \
	'while [ "$#" -gt 0 ]; do' \
	'  case "$1" in' \
	'    --target) target=$2; shift 2 ;;' \
	'    --target-dir) target_dir=$2; shift 2 ;;' \
	'    *) shift ;;' \
	'  esac' \
	'done' \
	'case "$target" in' \
	'  x86_64-pc-windows-msvc|aarch64-pc-windows-msvc) archive=ugoira_rs.lib ;;' \
	'  *) archive=libugoira_rs.a ;;' \
	'esac' \
	'mkdir -p "$target_dir/$target/release"' \
	'printf "temporary test archive for %s\\n" "$target" > "$target_dir/$target/release/$archive"' \
	> "$temporary/bin/cargo"
chmod 0755 "$temporary/bin/cargo"

PATH="$temporary/bin:$PATH" \
PIXIV_UGOIRA_STATICLIB_DIR="$temporary/staticlib" \
PIXIV_UGOIRA_TARGET_DIR="$temporary/cargo-target" \
sh "$repo_root/scripts/build-staticlibs.sh"

[ -f "$temporary/staticlib/manifest.json" ]
[ "$(jq '.artifacts | length' "$temporary/staticlib/manifest.json")" = 6 ]
jq -e '.artifacts["darwin/arm64"].path == "aarch64-apple-darwin/libugoira_rs.a"' "$temporary/staticlib/manifest.json" >/dev/null
jq -e '.artifacts["windows/amd64"].path == "x86_64-pc-windows-msvc/ugoira_rs.lib"' "$temporary/staticlib/manifest.json" >/dev/null

if PATH="$temporary/bin:$PATH" sh "$repo_root/scripts/build-staticlibs.sh" --target made-up-target >/dev/null 2>&1; then
	echo 'unsupported target unexpectedly succeeded' >&2
	exit 1
fi
