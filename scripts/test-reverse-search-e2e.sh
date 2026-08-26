#!/usr/bin/env bash
# 运行显式授权的真实反向搜图兼容性测试；source/key 只由测试进程读取，脚本不打印它们。
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  scripts/test-reverse-search-e2e.sh

The real reverse-search test is never part of the default test suite. Set
PIXIV_REVERSE_SEARCH_E2E=1 and provide PIXIV_REVERSE_SEARCH_SOURCE explicitly.
PIXIV_REVERSE_SEARCH_PROVIDER defaults to all; selecting saucenao or all also
requires SAUCENAO_API_KEY. PIXIV_REVERSE_SEARCH_PROXY is optional.
The source and API key are passed through to the test process and are never
printed by this script.
EOF
}

if (( $# > 1 )); then
	usage >&2
	exit 2
fi
if (( $# == 1 )); then
	case "$1" in
	--help|-h)
		usage
		exit 0
		;;
	*)
		usage >&2
		exit 2
		;;
	esac
fi

if [[ "${PIXIV_REVERSE_SEARCH_E2E:-}" != "1" ]]; then
	usage >&2
	exit 2
fi

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
exec go test ./e2e -run '^TestRealReverseSearch$' -count=1 -v
