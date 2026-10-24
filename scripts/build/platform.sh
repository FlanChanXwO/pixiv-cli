#!/bin/sh
# 为单个目标平台构建并可选打包 pixiv；发布身份与打包规则由 scripts/release 与 package.sh 持有。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)

usage() {
	cat >&2 <<'USAGE'
usage: scripts/build/platform.sh --target GOOS/GOARCH --output-dir DIR [--version VERSION] [--package]

Builds one native pixiv binary for an exact platform. When --package is set,
VERSION is required and the canonical release archive path is printed instead.
USAGE
}

fail() {
	printf 'build platform: %s\n' "$*" >&2
	exit 1
}

target=
version=
output_dir=
package=false

while [ "$#" -gt 0 ]; do
	case "$1" in
		--target|--version|--output-dir)
			[ "$#" -ge 2 ] || fail "$1 requires a value"
			case "$1" in
				--target) target=$2 ;;
				--version) version=$2 ;;
				--output-dir) output_dir=$2 ;;
			esac
			shift 2
			;;
		--package)
			package=true
			shift
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

[ -n "$target" ] || fail '--target is required'
[ -n "$output_dir" ] || fail '--output-dir is required'
case "$target" in
	*/*) goos=${target%%/*}; goarch=${target#*/} ;;
	*) fail '--target must use GOOS/GOARCH form' ;;
esac
[ -n "$goos" ] && [ -n "$goarch" ] || fail '--target must contain non-empty GOOS and GOARCH'
case "$goarch" in */*) fail '--target must contain exactly one slash' ;; esac

if [ "$package" = true ] && [ -z "$version" ]; then
	fail '--package requires --version'
fi
if [ -n "$version" ]; then
	(cd "$repo_root" && go run ./scripts/release assets validate --version "$version") >&2
fi

mkdir -p "$output_dir"
binary="$output_dir/pixiv"
if [ "$goos" = windows ]; then
	binary="$output_dir/pixiv.exe"
fi

if [ -n "$version" ]; then
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
		go build -trimpath -buildvcs=false \
		-ldflags "-X github.com/FlanChanXwO/pixiv-cli/internal/version.Version=v${version}" \
		-o "$binary" ./cmd/pixiv >&2
else
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
		go build -trimpath -buildvcs=false -o "$binary" ./cmd/pixiv >&2
fi

if [ "$package" != true ]; then
	printf '%s\n' "$binary"
	exit 0
fi

go run ./scripts/release assets package \
	--repo-root "$repo_root" \
	--version "$version" \
	--target "$target" \
	--binary "$binary" \
	--output-dir "$output_dir" >&2

case "$goos" in
	windows) artifact="$output_dir/pixiv-cli_${version}_${goos}_${goarch}.zip" ;;
	*) artifact="$output_dir/pixiv-cli_${version}_${goos}_${goarch}.tar.gz" ;;
esac
[ -f "$artifact" ] || fail "canonical package was not created: $artifact"
printf '%s\n' "$artifact"
