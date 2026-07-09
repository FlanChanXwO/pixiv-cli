#!/bin/sh
set -eu

goos=$(go env GOOS)
out=build/pixiv

if [ "$goos" = "windows" ]; then
	out=build/pixiv.exe
fi

mkdir -p build

printf 'GOOS=%s go build -trimpath -o %s ./cmd/pixiv\n' "$goos" "$out"
go build -trimpath -o "$out" ./cmd/pixiv
printf 'built %s\n' "$out"
