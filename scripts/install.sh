#!/bin/sh
set -eu

repository='FlanChanXwO/pixiv-cli'
release_root="https://github.com/$repository/releases/latest/download"
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

archive="$temporary/$asset"
printf 'Downloading %s...\n' "$asset"
curl -fsSL "$release_root/$asset" -o "$archive" || fail 'cannot download the platform archive from the official release'

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
