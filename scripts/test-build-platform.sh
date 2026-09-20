#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-platform-build-test.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
mkdir -p "$temporary/bin" "$temporary/staticlib"

real_go=$(command -v go)
version=0.0.0-platform-build-test
build_log="$temporary/build.log"
abi_log="$temporary/abi.log"

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
	'mkdir -p "$target_dir/$target/release"' \
	'printf "fixture staticlib\n" > "$target_dir/$target/release/libugoira_rs.a"' \
	> "$temporary/bin/cargo"
chmod 0755 "$temporary/bin/cargo"

cat > "$temporary/bin/go" <<'EOF'
#!/bin/sh
set -eu
case "$1" in
	env)
		exec "$PIXIV_TEST_REAL_GO" "$@"
		;;
	build)
		printf '%s|%s|%s\n' "${GOOS:-}" "${GOARCH:-}" "${CC:-}" > "$PIXIV_TEST_BUILD_LOG"
		output=
		while [ "$#" -gt 0 ]; do
			if [ "$1" = -o ]; then output=$2; shift 2; continue; fi
			shift
		done
		test -n "$output"
		cat > "$output" <<SCRIPT
#!/bin/sh
if [ "\${1:-}" = --version ]; then
  printf '%s\n' 'pixiv v$PIXIV_TEST_VERSION'
fi
SCRIPT
		chmod 0755 "$output"
		;;
	run)
		if [ "$2" = ./scripts/cmd/linuxabi ]; then
			printf '%s\n' "$*" > "$PIXIV_TEST_ABI_LOG"
			exit 0
		fi
		exec "$PIXIV_TEST_REAL_GO" "$@"
		;;
	*)
		exec "$PIXIV_TEST_REAL_GO" "$@"
		;;
esac
EOF
chmod 0755 "$temporary/bin/go"

printf '%s\n' 'sentinel manifest' > "$temporary/staticlib/manifest.json"
cp "$temporary/staticlib/manifest.json" "$temporary/manifest.before"

artifact=$(
	PATH="$temporary/bin:$PATH" \
	PIXIV_TEST_REAL_GO="$real_go" \
	PIXIV_TEST_VERSION="$version" \
	PIXIV_TEST_BUILD_LOG="$build_log" \
	PIXIV_TEST_ABI_LOG="$abi_log" \
	PIXIV_UGOIRA_STATICLIB_DIR="$temporary/staticlib" \
	PIXIV_UGOIRA_TARGET_DIR="$temporary/cargo-target" \
	sh "$repo_root/scripts/build-platform.sh" \
		--target linux/amd64 \
		--rust-target x86_64-unknown-linux-gnu \
		--cc gcc \
		--version "$version" \
		--output-dir "$temporary/dist" \
		--package
)

expected="$temporary/dist/pixiv-cli_${version}_linux_amd64.tar.gz"
[ "$artifact" = "$expected" ]
[ -f "$artifact" ]
cmp -s "$temporary/staticlib/manifest.json" "$temporary/manifest.before"
[ "$(cat "$build_log")" = 'linux|amd64|gcc' ]
grep -F './scripts/cmd/linuxabi --binary' "$abi_log" >/dev/null

mkdir "$temporary/extract"
tar -xzf "$artifact" -C "$temporary/extract"
[ "$("$temporary/extract/pixiv" --version)" = "pixiv v$version" ]

mkdir -p "$temporary/symlink-staticlib" "$temporary/leak-check"
ln -s "$temporary/manifest.before" "$temporary/symlink-staticlib/manifest.json"
if TMPDIR="$temporary/leak-check" \
	PATH="$temporary/bin:$PATH" \
	PIXIV_TEST_REAL_GO="$real_go" \
	PIXIV_UGOIRA_STATICLIB_DIR="$temporary/symlink-staticlib" \
	sh "$repo_root/scripts/build-platform.sh" \
		--target linux/amd64 \
		--rust-target x86_64-unknown-linux-gnu \
		--cc gcc \
		--output-dir "$temporary/symlink-output" >"$temporary/symlink.out" 2>&1; then
	echo 'symlink manifest unexpectedly accepted' >&2
	exit 1
fi
[ -z "$(find "$temporary/leak-check" -type f -print)" ]
