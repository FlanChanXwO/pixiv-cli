#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# 归档器会拒绝到根目录之间的任何 symlink；测试目录放在真实 repo 路径下，避免 macOS
# `/var -> /private/var` 系统 symlink 干扰被测行为。
temporary=$(mktemp -d "$repo_root/.pixiv-package-release-test.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

binary="$temporary/pixiv"
printf '#!/bin/sh\nprintf fixture\n' > "$binary"
chmod 0755 "$binary"

expected="$temporary/expected.txt"
{
	printf '%s\n' pixiv LICENSE THIRD_PARTY_LICENSES.md
	find "$repo_root/third_party/licenses" -type f -print | sed "s#^$repo_root/##"
} | LC_ALL=C sort > "$expected"

assert_members() {
	archive=$1
	format=$2
	actual="$temporary/actual-$format.txt"
	case "$format" in
		tar.gz)
			tar -tzf "$archive" | while IFS= read -r entry; do
				case "$entry" in */) ;; *) printf '%s\n' "$entry" ;; esac
			done | LC_ALL=C sort > "$actual"
			;;
		zip)
			unzip -Z1 "$archive" | while IFS= read -r entry; do
				case "$entry" in */) ;; *) printf '%s\n' "$entry" ;; esac
			done | LC_ALL=C sort > "$actual"
			;;
	esac
	diff -u "$expected" "$actual"
}

# Git Bash 默认会把无法创建的 Windows symlink 伪装成普通文本文件；这会让 `-L` 无法表达
# package-release 要拒绝的真实 reparse-point 边界。nativestrict 要求 MSYS 创建原生链接，失败即暴露
# runner 权限/配置问题，不会把安全测试静默降级为普通文件路径测试。
create_checked_symlink() {
	MSYS=winsymlinks:nativestrict ln -s "$1" "$2"
	[ -L "$2" ] || {
		echo "test symlink is not a native link: $2" >&2
		exit 1
	}
}

for format in tar.gz zip; do
	archive="$temporary/pixiv.$format"
	sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format "$format" --output "$archive"
	assert_members "$archive" "$format"
done

# GitHub 的 Windows runner 由 7-Zip 产出 zip；伪造 uname/7z 既验证该分支确实被选择，
# 也验证它仍写出与其余平台相同的 release member 集合。Windows runner 没有 `zip`，所以
# 优先委托其预装的真实 7z；非 Windows 开发机则保留 zip fixture，避免把本地工具差异伪装为
# package-release 的失败。
windows_bin="$temporary/windows-bin"
mkdir "$windows_bin"
windows_7z_marker="$temporary/windows-7z-used"
real_7z=$(command -v 7z || true)
printf '%s\n' '#!/bin/sh' 'printf "%s\\n" MINGW64_NT-10.0' > "$windows_bin/uname"
printf '%s\n' \
	'#!/bin/sh' \
	': > "$PIXIV_TEST_7Z_MARKER"' \
	'[ "$1" = a ] && [ "$2" = -tzip ] && [ "$3" = -bd ] || exit 64' \
	'if [ -n "${PIXIV_TEST_REAL_7Z:-}" ]; then' \
	'  exec "$PIXIV_TEST_REAL_7Z" "$@"' \
	'fi' \
	'output=$4' \
	'shift 4' \
	'zip -q -r "$output" "$@"' > "$windows_bin/7z"
chmod 0755 "$windows_bin/uname" "$windows_bin/7z"
windows_archive="$temporary/pixiv-windows.zip"
PIXIV_TEST_7Z_MARKER="$windows_7z_marker" PIXIV_TEST_REAL_7Z="$real_7z" PATH="$windows_bin:$PATH" \
	sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format zip --output "$windows_archive"
[ -f "$windows_7z_marker" ] || {
	echo 'Windows zip packaging did not invoke 7z' >&2
	exit 1
}
assert_members "$windows_archive" zip

if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format unsupported --output "$temporary/bad" >/dev/null 2>&1; then
	echo 'unsupported archive format unexpectedly succeeded' >&2
	exit 1
fi

existing_directory="$temporary/existing-output"
mkdir "$existing_directory"
printf 'sentinel\n' > "$existing_directory/keep"
if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format tar.gz --output "$existing_directory" >/dev/null 2>&1; then
	echo 'existing output directory unexpectedly succeeded' >&2
	exit 1
fi
if [ "$(find "$existing_directory" -type f | wc -l | tr -d ' ')" != 1 ]; then
	echo 'existing output directory received a residual archive' >&2
	exit 1
fi

output_link="$temporary/output-link"
create_checked_symlink "$existing_directory" "$output_link"
if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format tar.gz --output "$output_link/pixiv.tar.gz" >/dev/null 2>&1; then
	echo 'symlinked output directory unexpectedly succeeded' >&2
	exit 1
fi
if [ "$(find "$existing_directory" -type f | wc -l | tr -d ' ')" != 1 ]; then
	echo 'symlinked output directory received a residual archive' >&2
	exit 1
fi

real_child="$existing_directory/real-child"
mkdir "$real_child"
printf 'child sentinel\n' > "$real_child/keep"
ancestor_link="$temporary/ancestor-link"
create_checked_symlink "$existing_directory" "$ancestor_link"
if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format tar.gz --output "$ancestor_link/real-child/pixiv.tar.gz" >/dev/null 2>&1; then
	echo 'output below symlinked ancestor unexpectedly succeeded' >&2
	exit 1
fi
if [ "$(find "$real_child" -type f | wc -l | tr -d ' ')" != 1 ]; then
	echo 'output below symlinked ancestor received a residual archive' >&2
	exit 1
fi
