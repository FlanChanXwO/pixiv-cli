#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# 归档器拒绝 symlink ancestor；macOS 的 /var 指向 /private/var，fixture 使用真实仓库路径。
mkdir -p "$repo_root/build"
temporary=$(mktemp -d "$repo_root/build/pixiv-platform-build-test.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
mkdir -p "$temporary/bin"

real_go=$(command -v go)
version=0.0.0-platform-build-test
build_log="$temporary/build.log"
abi_log="$temporary/abi.log"

cat > "$temporary/bin/go" <<'EOF'
#!/bin/sh
set -eu
case "$1" in
	env)
		exec "$PIXIV_TEST_REAL_GO" "$@"
		;;
	build)
		printf '%s|%s|%s\n' "${GOOS:-}" "${GOARCH:-}" "${CGO_ENABLED:-}" > "$PIXIV_TEST_BUILD_LOG"
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

artifact=$(
	PATH="$temporary/bin:$PATH" \
	PIXIV_TEST_REAL_GO="$real_go" \
	PIXIV_TEST_VERSION="$version" \
	PIXIV_TEST_BUILD_LOG="$build_log" \
	PIXIV_TEST_ABI_LOG="$abi_log" \
	sh "$repo_root/scripts/build-platform.sh" \
		--target linux/amd64 \
		--version "$version" \
		--output-dir "$temporary/dist" \
		--package
)

expected="$temporary/dist/pixiv-cli_${version}_linux_amd64.tar.gz"
[ "$artifact" = "$expected" ]
[ -f "$artifact" ]
[ "$(cat "$build_log")" = 'linux|amd64|0' ]
grep -F './scripts/cmd/linuxabi --binary' "$abi_log" >/dev/null

mkdir "$temporary/extract"
tar -xzf "$artifact" -C "$temporary/extract"
[ "$("$temporary/extract/pixiv" --version)" = "pixiv v$version" ]

# Windows keeps its executable suffix and ZIP packaging without a C linker.
artifact=$(
	PATH="$temporary/bin:$PATH" \
	PIXIV_TEST_REAL_GO="$real_go" \
	PIXIV_TEST_VERSION="$version" \
	PIXIV_TEST_BUILD_LOG="$build_log" \
	PIXIV_TEST_ABI_LOG="$abi_log" \
	sh "$repo_root/scripts/build-platform.sh" \
		--target windows/arm64 \
		--version "$version" \
		--output-dir "$temporary/windows" \
		--package
)
[ "$artifact" = "$temporary/windows/pixiv-cli_${version}_windows_arm64.zip" ]
[ "$(cat "$build_log")" = 'windows|arm64|0' ]
unzip -Z1 "$artifact" | grep -x pixiv.exe >/dev/null
