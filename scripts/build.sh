#!/bin/sh
# 用本机 GOOS/GOARCH 构建 CGO-free 的 pixiv 二进制到 build/。
set -eu

goos=$(go env GOOS)
out=build/pixiv

if [ "$goos" = "windows" ]; then
	out=build/pixiv.exe
fi

mkdir -p build

printf 'CGO_ENABLED=0 GOOS=%s GOARCH=%s go build -trimpath -o %s ./cmd/pixiv\n' "$goos" "$(go env GOARCH)" "$out"
CGO_ENABLED=0 go build -trimpath -o "$out" ./cmd/pixiv
printf 'built %s\n' "$out"
