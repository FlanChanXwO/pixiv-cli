#!/bin/sh
# 将已构建的单个 pixiv binary 与完整许可证集合封装为 release archive。
set -eu

# macOS 的 cp 会把二进制的扩展属性转成 AppleDouble `._*` 文件；这些文件既不属于
# release contract，也会让 runner evidence 的 archive member 清单与许可证树失配。
# 关闭 copyfile metadata，确保 tar/zip 只包含显式列出的常规文件。
export COPYFILE_DISABLE=1

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
binary=
output=
format=

usage() {
	cat >&2 <<'EOF'
usage: scripts/package-release.sh --binary PATH --format tar.gz|zip --output PATH

Packages exactly one supplied binary, LICENSE, THIRD_PARTY_LICENSES.md, and the
complete third_party/licenses tree. This command does not sign, checksum, name,
or upload release assets; those release-policy steps are intentionally separate.
EOF
}

fail() {
	printf '%s\n' "package release: $*" >&2
	exit 1
}

# 检查用户提供输出目录到根目录的每个既有祖先。不能使用 realpath，因为它会在检查前
# 跟随 symlink；以词法路径逐级 -L 才能阻止 link/real-child/file 这类穿透。
reject_output_symlink_ancestors() {
	case "$1" in
		/*) ancestor=$1 ;;
		*) ancestor=$PWD/$1 ;;
	esac
	while :; do
		[ ! -L "$ancestor" ] || fail "output directory contains a symlink ancestor: $ancestor"
		parent=$(dirname -- "$ancestor")
		[ "$parent" = "$ancestor" ] && break
		ancestor=$parent
	done
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--binary)
			[ "$#" -ge 2 ] || fail '--binary requires a path'
			binary=$2
			shift 2
			;;
		--format)
			[ "$#" -ge 2 ] || fail '--format requires tar.gz or zip'
			format=$2
			shift 2
			;;
		--output)
			[ "$#" -ge 2 ] || fail '--output requires a path'
			output=$2
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

[ -n "$binary" ] || fail '--binary is required'
[ -n "$output" ] || fail '--output is required'
case "$format" in
	tar.gz|zip) ;;
	*) fail "unsupported archive format: $format" ;;
esac

[ -f "$binary" ] || fail "binary is not a regular file: $binary"
[ ! -L "$binary" ] || fail "binary must not be a symlink: $binary"
[ -f "$repo_root/LICENSE" ] || fail 'root LICENSE is missing'
[ -f "$repo_root/THIRD_PARTY_LICENSES.md" ] || fail 'THIRD_PARTY_LICENSES.md is missing'
[ -d "$repo_root/third_party/licenses" ] || fail 'third_party/licenses is missing'
if find "$repo_root/third_party/licenses" -type l -print -quit | grep -q .; then
	fail 'third_party/licenses must not contain symlinks'
fi
if ! find "$repo_root/third_party/licenses" -type f -print -quit | grep -q .; then
	fail 'third_party/licenses contains no license files'
fi

output_parent=$(dirname -- "$output")
[ ! -d "$output" ] || fail "output must be a file path, not an existing directory: $output"
[ ! -L "$output" ] || fail "output must not be a symlink: $output"
[ -d "$output_parent" ] || fail "output directory does not exist: $output_parent"
reject_output_symlink_ancestors "$output_parent"
output_dir=$(CDPATH= cd -- "$output_parent" && pwd)
output_base=$(basename -- "$output")
stage=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-release-package.XXXXXX")
temporary_output=$(mktemp "$output_dir/.${output_base}.XXXXXX")
cleanup() {
	rm -rf "$stage"
	rm -f "$temporary_output"
}
trap cleanup EXIT HUP INT TERM

binary_name=$(basename -- "$binary")
cp "$binary" "$stage/$binary_name"
cp "$repo_root/LICENSE" "$stage/LICENSE"
cp "$repo_root/THIRD_PARTY_LICENSES.md" "$stage/THIRD_PARTY_LICENSES.md"
mkdir -p "$stage/third_party"
cp -R "$repo_root/third_party/licenses" "$stage/third_party/licenses"
# macOS zip 拒绝覆盖 mktemp 创建的空文件；先移除占位，再由打包器独占创建输出。
rm -f "$temporary_output"

case "$format" in
	tar.gz)
		tar -C "$stage" -czf "$temporary_output" "$binary_name" LICENSE THIRD_PARTY_LICENSES.md third_party/licenses
		;;
	zip)
		(
			cd "$stage"
			# GitHub Windows runner 的 Git Bash 没有 zip；该镜像通过 Chocolatey 预装 7-Zip。
			# 不能退回到不确定的工具或跳过归档：缺少约定工具应让 native evidence 明确失败。
			case "$(uname -s)" in
				MINGW*|MSYS*|CYGWIN*)
					command -v 7z >/dev/null 2>&1 || fail 'Windows zip packaging requires runner-provided 7z'
					7z a -tzip -bd "$temporary_output" "$binary_name" LICENSE THIRD_PARTY_LICENSES.md third_party/licenses
					;;
				*)
					command -v zip >/dev/null 2>&1 || fail 'zip packaging requires zip'
					zip -q -r "$temporary_output" "$binary_name" LICENSE THIRD_PARTY_LICENSES.md third_party/licenses
					;;
			esac
		)
		;;
esac
mv -f "$temporary_output" "$output"
printf 'packaged %s\n' "$output"
