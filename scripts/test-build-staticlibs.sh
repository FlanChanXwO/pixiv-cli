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

PATH="$temporary/bin:$PATH" \
PIXIV_UGOIRA_STATICLIB_DIR="$temporary/staticlib" \
PIXIV_UGOIRA_TARGET_DIR="$temporary/cargo-target" \
sh "$repo_root/scripts/build-staticlibs.sh" --target aarch64-apple-darwin
if [ -e "$temporary/staticlib/manifest.json" ]; then
	echo 'single-target rebuild retained or published a potentially stale manifest' >&2
	exit 1
fi

tracked_manifest="$repo_root/internal/downloader/ugoira_rs/staticlib/manifest.json"
[ -f "$tracked_manifest" ]
tracked_manifest_hash_before=$(shasum -a 256 "$tracked_manifest" | awk '{print $1}')

invalid_staticlib_dir="$temporary/invalid-staticlib"
invalid_target_dir="$temporary/invalid-cargo-target"
mkdir -p "$invalid_staticlib_dir"
printf '%s\n' 'sentinel manifest that an invalid target must not invalidate' > "$invalid_staticlib_dir/manifest.json"
sentinel_snapshot="$temporary/sentinel-manifest.snapshot"
cp "$invalid_staticlib_dir/manifest.json" "$sentinel_snapshot"
sentinel_hash_before=$(shasum -a 256 "$invalid_staticlib_dir/manifest.json" | awk '{print $1}')

if PATH="$temporary/bin:$PATH" \
	PIXIV_UGOIRA_STATICLIB_DIR="$invalid_staticlib_dir" \
	PIXIV_UGOIRA_TARGET_DIR="$invalid_target_dir" \
	sh "$repo_root/scripts/build-staticlibs.sh" --target made-up-target >"$temporary/invalid-target.out" 2>&1; then
	echo 'unsupported target unexpectedly succeeded' >&2
	exit 1
fi
if [ ! -f "$invalid_staticlib_dir/manifest.json" ]; then
	echo 'unsupported target invalidated the sentinel manifest before validation' >&2
	exit 1
fi
cmp -s "$invalid_staticlib_dir/manifest.json" "$sentinel_snapshot"
[ "$(shasum -a 256 "$invalid_staticlib_dir/manifest.json" | awk '{print $1}')" = "$sentinel_hash_before" ]
[ "$(shasum -a 256 "$tracked_manifest" | awk '{print $1}')" = "$tracked_manifest_hash_before" ]
[ ! -e "$invalid_target_dir" ]
grep -F 'unsupported Rust target made-up-target; expected one of the six release targets' "$temporary/invalid-target.out" >/dev/null

if PATH="$temporary/bin:$PATH" \
	PIXIV_UGOIRA_STATICLIB_DIR="$invalid_staticlib_dir" \
	PIXIV_UGOIRA_TARGET_DIR="$invalid_target_dir" \
	sh "$repo_root/scripts/build-staticlibs.sh" --target '' >"$temporary/empty-target.out" 2>&1; then
	echo 'empty explicit target unexpectedly succeeded' >&2
	exit 1
fi
cmp -s "$invalid_staticlib_dir/manifest.json" "$sentinel_snapshot"
[ "$(shasum -a 256 "$invalid_staticlib_dir/manifest.json" | awk '{print $1}')" = "$sentinel_hash_before" ]
[ "$(shasum -a 256 "$tracked_manifest" | awk '{print $1}')" = "$tracked_manifest_hash_before" ]
[ ! -e "$invalid_target_dir" ]
grep -F 'unsupported Rust target ; expected one of the six release targets' "$temporary/empty-target.out" >/dev/null
