#!/bin/sh
set -eu

goos=$(go env GOOS)
goarch=$(go env GOARCH)
out=build/pixiv

if [ "$goos" = "windows" ]; then
	out=build/pixiv.exe
fi

[ "$(go env GOVERSION)" = 'go1.26.3' ] || {
	printf 'Go 1.26.3 is required to build pixiv-cli with the committed Rust ugoira staticlib (found %s)\n' "$(go env GOVERSION)" >&2
	exit 1
}
[ "$(go env CGO_ENABLED)" = '1' ] || {
	printf 'Go 1.26.3 with CGO_ENABLED=1 and a working target C linker is required to link the committed Rust ugoira staticlib\n' >&2
	exit 1
}
cc=$(go env CC)
command -v "$cc" >/dev/null 2>&1 || {
	printf 'Go 1.26.3 with CGO_ENABLED=1 requires a working target C linker; Go selected CC=%s\n' "$cc" >&2
	exit 1
}
case "$goos/$goarch" in
	darwin/amd64|darwin/arm64|linux/amd64|linux/arm64|windows/amd64|windows/arm64) ;;
	*)
	printf 'Rust ugoira staticlib is unavailable for %s/%s; Go 1.26.3 with CGO_ENABLED=1 and a supported target C linker are required\n' "$goos" "$goarch" >&2
	exit 1
	;;
esac
[ -f internal/downloader/ugoira_rs/staticlib/manifest.json ] || {
	printf 'committed six-target Rust staticlib manifest is missing; source/release builds require all native artifacts before Go 1.26.3+cgo can link them\n' >&2
	exit 1
}

printf 'go test ./internal/downloader -run ^TestCommittedUgoiraStaticlibManifestWhenPresent$ -count=1\n'
go test ./internal/downloader -run '^TestCommittedUgoiraStaticlibManifestWhenPresent$' -count=1

mkdir -p build

printf 'GOOS=%s GOARCH=%s go build -trimpath -o %s ./cmd/pixiv\n' "$goos" "$goarch" "$out"
go build -trimpath -o "$out" ./cmd/pixiv
printf 'built %s\n' "$out"
