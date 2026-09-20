#!/bin/sh
set -eu

target="$(go env GOOS)/$(go env GOARCH)"

if [ "$target" != "windows/arm64" ]; then
	exec go test -race ./...
fi

# Go 1.27.1 不支持 windows/arm64 race detector。该平台仍必须实际执行 gate：
# 只接受 Go 官方明确的“不支持”诊断，其他任何测试/工具链错误都保持失败。
output=$(mktemp)
trap 'rm -f "$output"' EXIT HUP INT TERM

if go test -race ./... >"$output" 2>&1; then
	cat "$output"
	printf '%s\n' 'go test -race unexpectedly succeeded on windows/arm64' >&2
	exit 1
fi

cat "$output"
grep -Fx -- '-race is not supported on windows/arm64' "$output" >/dev/null || {
	printf '%s\n' 'go test -race failed for an unexpected reason on windows/arm64' >&2
	exit 1
}
