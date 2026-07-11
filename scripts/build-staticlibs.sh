#!/bin/sh
# 受控生成 Rust ugoira staticlib。该脚本绝不把某个平台的库伪装成另一个平台的产物。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
crate_manifest="$repo_root/internal/download/ugoira_rs/Cargo.toml"
staticlib_dir=${PIXIV_UGOIRA_STATICLIB_DIR:-"$repo_root/internal/download/ugoira_rs/staticlib"}
target_dir=${PIXIV_UGOIRA_TARGET_DIR:-}
target_arg=

usage() {
	cat >&2 <<'EOF'
usage: scripts/build-staticlibs.sh [--target RUST_TARGET]

Builds the Rust ugoira static library using Cargo's locked offline dependency set.
Without --target, builds all six release targets. A manifest is written only after
all six real target artifacts have been produced. Cross targets require the Rust
target and its matching C linker to be installed on the current runner.
EOF
}

fail() {
	printf '%s\n' "build staticlibs: $*" >&2
	exit 1
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--target)
			[ "$#" -ge 2 ] || fail '--target requires a Rust target triple'
			[ -z "$target_arg" ] || fail 'only one --target may be supplied per invocation'
			target_arg=$2
			shift 2
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			usage
			fail "unknown argument: $1"
			;;
	esac
done

command -v cargo >/dev/null 2>&1 || fail 'cargo is required to build Rust staticlibs'
command -v go >/dev/null 2>&1 || fail 'Go 1.26.3 is required to verify the Rust source digest'
[ "$(go env GOVERSION)" = 'go1.26.3' ] || fail "Go 1.26.3 is required, found $(go env GOVERSION)"
[ -f "$crate_manifest" ] || fail "Rust crate manifest is missing: $crate_manifest"

targets='x86_64-apple-darwin aarch64-apple-darwin x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu x86_64-pc-windows-msvc aarch64-pc-windows-msvc'
if [ -n "$target_arg" ]; then
	targets=$target_arg
fi

target_kind() {
	case "$1" in
		x86_64-apple-darwin|aarch64-apple-darwin|x86_64-unknown-linux-gnu|aarch64-unknown-linux-gnu)
			printf '%s\n' 'libugoira_rs.a'
			;;
		x86_64-pc-windows-msvc|aarch64-pc-windows-msvc)
			printf '%s\n' 'ugoira_rs.lib'
			;;
		*)
			fail "unsupported Rust target $1; expected one of the six release targets"
			;;
	esac
}

temporary_target_dir=
if [ -z "$target_dir" ]; then
	target_dir=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-ugoira-staticlib.XXXXXX")
	temporary_target_dir=$target_dir
	trap 'rm -rf "$temporary_target_dir"' EXIT HUP INT TERM
fi

# 单 target build 不能证明其余五个库与当前 Rust source 同代；先废止旧 manifest，
# 并且本次绝不重新发布它，避免把混代库伪装成受验证的六平台集合。
if [ -n "$target_arg" ] && { [ -e "$staticlib_dir/manifest.json" ] || [ -L "$staticlib_dir/manifest.json" ]; }; then
	rm -f "$staticlib_dir/manifest.json" || fail "cannot invalidate stale staticlib manifest: $staticlib_dir/manifest.json"
	printf 'invalidated %s/manifest.json before single-target rebuild\n' "$staticlib_dir" >&2
fi

for target in $targets; do
	archive=$(target_kind "$target")
	printf 'cargo build --locked --offline --release --target %s --target-dir %s --manifest-path %s\n' "$target" "$target_dir" "$crate_manifest"
	if ! cargo build --locked --offline --release --target "$target" --target-dir "$target_dir" --manifest-path "$crate_manifest"; then
		fail "failed to build $target; install that Rust target and its matching C linker before retrying"
	fi
	source="$target_dir/$target/release/$archive"
	[ -f "$source" ] || fail "Cargo reported success for $target but expected archive is absent: $source"
	[ ! -L "$source" ] || fail "Cargo produced a symlink instead of the expected archive: $source"
	destination_dir="$staticlib_dir/$target"
	mkdir -p "$destination_dir"
	temporary_destination=$(mktemp "$destination_dir/.${archive}.XXXXXX")
	cp "$source" "$temporary_destination"
	chmod 0644 "$temporary_destination"
	mv -f "$temporary_destination" "$destination_dir/$archive"
	printf 'wrote %s\n' "$destination_dir/$archive"
done

if [ -n "$target_arg" ]; then
	printf '%s\n' 'manifest not written: a single-target build cannot prove a same-source six-target staticlib generation' >&2
	exit 0
fi

all_artifacts_exist=true
for target in x86_64-apple-darwin aarch64-apple-darwin x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu x86_64-pc-windows-msvc aarch64-pc-windows-msvc; do
	archive=$(target_kind "$target")
	if [ ! -f "$staticlib_dir/$target/$archive" ] || [ -L "$staticlib_dir/$target/$archive" ]; then
		all_artifacts_exist=false
		break
	fi
done

if [ "$all_artifacts_exist" != true ]; then
	printf '%s\n' 'manifest not written: all six real staticlib artifacts are required' >&2
	exit 0
fi

(
	cd "$repo_root"
	go test ./internal/download -run '^TestUgoiraRustSourceDigestMatchesRecordedFixture$' -count=1
)
source_digest=$(tr -d '\r\n[:space:]' < "$repo_root/internal/download/testdata/ugoira-source-digest.txt")

manifest_temp=$(mktemp "$staticlib_dir/.manifest.json.XXXXXX")
{
	printf '{"schema":1,"source_digest":"%s","artifacts":{' "$source_digest"
	separator=
	for platform_target in \
		'darwin/amd64 x86_64-apple-darwin' \
		'darwin/arm64 aarch64-apple-darwin' \
		'linux/amd64 x86_64-unknown-linux-gnu' \
		'linux/arm64 aarch64-unknown-linux-gnu' \
		'windows/amd64 x86_64-pc-windows-msvc' \
		'windows/arm64 aarch64-pc-windows-msvc'; do
		platform=${platform_target%% *}
		target=${platform_target#* }
		archive=$(target_kind "$target")
		path="$target/$archive"
		digest=$(shasum -a 256 "$staticlib_dir/$path" | awk '{print $1}')
		printf '%s"%s":{"target":"%s","path":"%s","sha256":"%s"}' "$separator" "$platform" "$target" "$path" "$digest"
		separator=,
	done
	printf '}}\n'
} > "$manifest_temp"
chmod 0644 "$manifest_temp"
mv -f "$manifest_temp" "$staticlib_dir/manifest.json"
printf 'wrote %s/manifest.json after all six artifact hashes were verified\n' "$staticlib_dir"
