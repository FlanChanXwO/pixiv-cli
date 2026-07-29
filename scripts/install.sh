#!/bin/sh
set -eu

repository='FlanChanXwO/pixiv-cli'
release_root="https://github.com/$repository/releases/latest/download"
# 发布阶段会把 internal/update/release_sources.txt 注入此块。工作树模板保留直连，
# 以便审阅中的脚本不会擅自依赖公共中继。
# PIXIV_RELEASE_SOURCES_BEGIN
release_sources='github-direct|{url}|{url}'
# PIXIV_RELEASE_SOURCES_END
if [ -n "${PIXIV_RELEASE_SOURCES:-}" ]; then
	release_sources=$PIXIV_RELEASE_SOURCES
fi
user_home=${HOME:-}
install_dir=${PIXIV_INSTALL_DIR:-${user_home:+"$user_home/.local/bin"}}
path_mode=report
temporary=
staged=

usage() {
	cat <<'EOF'
Install the latest stable pixiv-cli release for this machine.

Usage:
  install.sh [--install-dir DIR] [--add-to-path|--no-path]

Options:
  --install-dir DIR  Install pixiv into DIR (default: $HOME/.local/bin).
  --add-to-path      Add $HOME/.local/bin to the current shell's user profile.
  --no-path          Do not modify shell profile files.
  -h, --help         Show this help.
EOF
}

fail() {
	printf 'pixiv installer: %s\n' "$1" >&2
	exit 1
}

cleanup() {
	if [ -n "$staged" ]; then
		rm -f -- "$staged"
	fi
	if [ -n "$temporary" ]; then
		rm -rf -- "$temporary"
	fi
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--install-dir)
		[ "$#" -ge 2 ] || fail '--install-dir requires a directory'
		install_dir=$2
		shift 2
		;;
	--add-to-path)
		path_mode=add
		shift
		;;
	--no-path)
		path_mode=skip
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*) fail "unknown argument: $1" ;;
	esac
done

[ -n "$install_dir" ] || fail 'install directory cannot be empty'
[ -n "$user_home" ] || fail 'HOME is required'
if [ "$path_mode" = add ] && [ "$install_dir" != "$user_home/.local/bin" ]; then
	fail '--add-to-path only supports the default $HOME/.local/bin directory'
fi
command -v curl >/dev/null 2>&1 || fail 'curl is required'
command -v tar >/dev/null 2>&1 || fail 'tar is required'
command -v awk >/dev/null 2>&1 || fail 'awk is required'
command -v cmp >/dev/null 2>&1 || fail 'cmp is required'
command -v mkfifo >/dev/null 2>&1 || fail 'mkfifo is required'
command -v od >/dev/null 2>&1 || fail 'od is required'

case "$(uname -s)" in
Linux) target_os=linux ;;
Darwin) target_os=darwin ;;
*) fail "unsupported operating system: $(uname -s)" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) target_arch=amd64 ;;
aarch64 | arm64) target_arch=arm64 ;;
*) fail "unsupported architecture: $(uname -m)" ;;
esac

temporary=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-install.XXXXXX") || fail 'cannot create a private temporary directory'
trap cleanup EXIT
trap 'exit 130' HUP INT TERM
checksums="$temporary/checksums.txt"

printf 'Downloading the latest stable release metadata...\n'
curl -fsSL "$release_root/checksums.txt" -o "$checksums" || fail 'cannot download checksums.txt from the official release'

suffix="_${target_os}_${target_arch}.tar.gz"
asset=$(awk -v suffix="$suffix" '$2 ~ suffix "$" { print $2 }' "$checksums")
[ "$(printf '%s\n' "$asset" | awk 'NF { count++ } END { print count + 0 }')" -eq 1 ] || fail "checksums.txt must contain exactly one $target_os/$target_arch archive"
case "$asset" in
pixiv-cli_*"$suffix") ;;
*) fail 'release metadata contains an unexpected archive name' ;;
esac
case "$asset" in
*/* | *\\*) fail 'release archive name must be a basename' ;;
esac

expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "$checksums")
[ "${#expected}" -eq 64 ] || fail 'release checksum is not a SHA-256 digest'
case "$expected" in
*[!0-9a-f]*) fail 'release checksum is not lowercase hexadecimal' ;;
esac

url_encode() {
	printf '%s' "$1" | LC_ALL=C od -An -tx1 | awk '{ for (index = 1; index <= NF; index++) printf "%%%s", toupper($index) }'
}

render_release_url() {
	template=$1
	canonical_url=$2
	case "$template" in
	*'{url_query}'*)
		prefix=${template%%\{url_query\}*}
		suffix=${template#*\{url_query\}}
		printf '%s%s%s' "$prefix" "$(url_encode "$canonical_url")" "$suffix"
		;;
	*'{url}'*)
		prefix=${template%%\{url\}*}
		suffix=${template#*\{url\}}
		printf '%s%s%s' "$prefix" "$canonical_url" "$suffix"
		;;
	*) return 1 ;;
	esac
}

select_asset_template() {
	probe_fifo="$temporary/release-source-probes"
	mkfifo "$probe_fifo" || fail 'cannot create release-source probe channel'
	# 同时探测所有候选。每个候选必须逐字返回直连得到的 checksums.txt；最先完成者
	# 成为本次下载首选，失败候选不会污染权威 checksum。
	probe_count=0
	probe_pids=
	exec 3<>"$probe_fifo"
	while IFS= read -r source_line; do
		case "$source_line" in
		'' | \#*) continue ;;
		esac
		source_id=${source_line%%|*}
		source_rest=${source_line#*|}
		[ "$source_rest" != "$source_line" ] || fail 'release source entry is malformed'
		source_api=${source_rest%%|*}
		source_template=${source_rest#*|}
		[ "$source_template" != "$source_rest" ] || fail 'release source entry is malformed'
		probe_url=$(render_release_url "$source_template" "$release_root/checksums.txt") || fail 'release source template is invalid'
		(
			probe_file="$temporary/probe-$probe_count.txt"
			if curl -fsSL "$probe_url" -o "$probe_file" && cmp -s "$checksums" "$probe_file"; then
				printf 'ok|%s|%s\n' "$source_id" "$source_template" >&3
			else
				printf 'failed|%s|%s\n' "$source_id" "$source_template" >&3
			fi
		) &
		probe_pids="$probe_pids $!"
		probe_count=$((probe_count + 1))
	done <<EOF
$release_sources
EOF
	[ "$probe_count" -gt 0 ] || fail 'release source list is empty'
	selected_template=
	selected_source=
	probe_index=0
	while [ "$probe_index" -lt "$probe_count" ]; do
		IFS='|' read -r probe_result probe_source probe_template <&3 || fail 'release-source probe did not report a result'
		if [ "$probe_result" = ok ] && [ -z "$selected_template" ]; then
			selected_template=$probe_template
			selected_source=$probe_source
			break
		fi
		probe_index=$((probe_index + 1))
	done
	# 选出最快可用候选后立即取消尚在探测的 curl；未选中时等待所有 probe 的失败报告。
	if [ -n "$selected_template" ]; then
		kill $probe_pids 2>/dev/null || true
	fi
	for probe_pid in $probe_pids; do
		wait "$probe_pid" 2>/dev/null || true
	done
	exec 3>&-
	exec 3<&-
	[ -n "$selected_template" ] || fail 'no release source returned the official checksums.txt'
}

select_asset_template
archive="$temporary/$asset"
printf 'Downloading %s...\n' "$asset"
download_archive_from_sources() {
	archive_canonical_url="$release_root/$asset"
	archive_url=$(render_release_url "$selected_template" "$archive_canonical_url") || fail 'release source template is invalid'
	if curl -fsSL "$archive_url" -o "$archive"; then
		return 0
	fi
	while IFS= read -r source_line; do
		case "$source_line" in
		'' | \#*) continue ;;
		esac
		source_id=${source_line%%|*}
		source_rest=${source_line#*|}
		[ "$source_rest" != "$source_line" ] || fail 'release source entry is malformed'
		source_api=${source_rest%%|*}
		source_template=${source_rest#*|}
		[ "$source_template" != "$source_rest" ] || fail 'release source entry is malformed'
		[ "$source_id" = "$selected_source" ] && continue
		archive_url=$(render_release_url "$source_template" "$archive_canonical_url") || fail 'release source template is invalid'
		if curl -fsSL "$archive_url" -o "$archive"; then
			return 0
		fi
	done <<EOF
$release_sources
EOF
	return 1
}

download_archive_from_sources || fail 'cannot download the platform archive from any release source'

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$archive" | awk '{ print $1 }')
else
	fail 'sha256sum or shasum is required to verify the download'
fi
[ "$actual" = "$expected" ] || fail 'SHA-256 mismatch; the existing installation was not changed'
printf 'SHA-256 verified.\n'

extract_dir="$temporary/extract"
mkdir "$extract_dir"
tar -xzf "$archive" -C "$extract_dir" pixiv || fail 'the verified archive does not contain pixiv'
[ -f "$extract_dir/pixiv" ] && [ ! -L "$extract_dir/pixiv" ] || fail 'the archive pixiv member is not a regular file'

mkdir -p "$install_dir" || fail 'cannot create the install directory'
staged=$(mktemp "$install_dir/.pixiv-install.XXXXXX") || fail 'cannot stage pixiv in the install directory'
cp "$extract_dir/pixiv" "$staged" || fail 'cannot stage the verified binary'
chmod 0755 "$staged" || fail 'cannot make the staged binary executable'
"$staged" version --json >/dev/null 2>&1 || fail 'the staged binary failed its version preflight'
target="$install_dir/pixiv"
mv -f "$staged" "$target" || fail 'cannot replace the installed binary'
staged=

# 官方 installer 主动初始化按需 pixiv:// handler。该 helper 的失败不会使已通过
# 校验的 binary 安装失效；binary 会给出明确 warning，首次正常 browser login 也会重试。
if ! "$target" auth _install-handler; then
	printf 'warning: pixiv callback handler initialization could not be started; run pixiv auth login once after installation.\n' >&2
fi

path_contains_install_dir=false
case ":${PATH:-}:" in
*":$install_dir:"*) path_contains_install_dir=true ;;
esac

if [ "$path_mode" = add ] && [ "$path_contains_install_dir" = false ]; then
	case "${SHELL##*/}" in
	zsh) profile="$HOME/.zshrc" ;;
	bash) profile="$HOME/.bashrc" ;;
	*) profile="$HOME/.profile" ;;
	esac
	path_line='export PATH="$HOME/.local/bin:$PATH"'
	if [ ! -f "$profile" ] || ! grep -Fqx "$path_line" "$profile"; then
		printf '\n%s\n' "$path_line" >>"$profile" || fail "cannot update $profile"
	fi
	printf 'Added $HOME/.local/bin to %s; open a new terminal to use it.\n' "$profile"
elif [ "$path_mode" != skip ] && [ "$path_contains_install_dir" = false ]; then
	printf 'Add %s to PATH, or rerun with --add-to-path when using $HOME/.local/bin.\n' "$install_dir"
fi

printf 'Installed pixiv to %s\n' "$target"
"$target" version
