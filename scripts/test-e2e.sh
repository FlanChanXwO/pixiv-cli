#!/usr/bin/env bash
# 运行完整真实 Pixiv E2E。所有认证输入只保留在当前进程环境，绝不写入文件或命令输出。
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  scripts/test-e2e.sh [--non-interactive] [REFRESH_TOKEN SFW_ILLUST_ID R18_ILLUST_ID R18_UGOIRA_ID ILLUST_SEARCH_WORD DISCOVERY_WORD [PROXY]]

Without positional inputs, values are read from the environment. Missing values are
prompted only when stdin is a TTY; the refresh token is read without echo.

Required variables:
  PIXIV_E2E_REFRESH_TOKEN
  PIXIV_E2E_SFW_ILLUST_ID
  PIXIV_E2E_R18_ILLUST_ID
  PIXIV_E2E_R18_UGOIRA_ID
  PIXIV_E2E_ILLUST_SEARCH_WORD
  PIXIV_E2E_DISCOVERY_WORD

Optional proxy:
  PIXIV_E2E_PROXY (also used as PIXIV_WEB_API_PROXY)
EOF
}

non_interactive=0
if [[ "${1:-}" == "--non-interactive" ]]; then
	non_interactive=1
	shift
fi
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
	usage
	exit 0
fi

if (( $# > 0 )); then
	if (( $# != 6 && $# != 7 )); then
		usage >&2
		exit 2
	fi
	PIXIV_E2E_REFRESH_TOKEN=$1
	PIXIV_E2E_SFW_ILLUST_ID=$2
	PIXIV_E2E_R18_ILLUST_ID=$3
	PIXIV_E2E_R18_UGOIRA_ID=$4
	PIXIV_E2E_ILLUST_SEARCH_WORD=$5
	PIXIV_E2E_DISCOVERY_WORD=$6
	PIXIV_E2E_PROXY=${7:-}
	PIXIV_WEB_API_PROXY=${7:-}
fi

require_value() {
	local name=$1
	local prompt=$2
	if [[ -n "${!name:-}" ]]; then
		return
	fi
	if (( non_interactive != 0 )); then
		printf '%s\n' "$name is required for a complete real E2E run" >&2
		exit 2
	fi
	if [[ ! -t 0 ]]; then
		printf '%s\n' "$name is required; set it or run this script from a TTY" >&2
		exit 2
	fi
	printf '%s: ' "$prompt" >&2
	if [[ "$name" == "PIXIV_E2E_REFRESH_TOKEN" ]]; then
		read -r -s PIXIV_E2E_REFRESH_TOKEN
		printf '\n' >&2
		return
	fi
	read -r "$name"
}

require_value PIXIV_E2E_REFRESH_TOKEN "Pixiv refresh token"
require_value PIXIV_E2E_SFW_ILLUST_ID "SFW illustration ID"
require_value PIXIV_E2E_R18_ILLUST_ID "R18 illustration ID"
require_value PIXIV_E2E_R18_UGOIRA_ID "R18 ugoira illustration ID"
require_value PIXIV_E2E_ILLUST_SEARCH_WORD "Illustration search word"
require_value PIXIV_E2E_DISCOVERY_WORD "Novel and user discovery word"

# 当前脚本始终测试由本次明确 token 驱动的源码构建产物，不读取本机账号 store，
# 也不允许宿主环境误将外部 release binary 混入真实 API 验收。
unset PIXIV_E2E_USE_LOCAL_AUTH PIXIV_E2E_BINARY PIXIV_E2E_EXPECTED_VERSION
export PIXIV_E2E_REFRESH_TOKEN
export PIXIV_E2E_SFW_ILLUST_ID PIXIV_E2E_R18_ILLUST_ID PIXIV_E2E_R18_UGOIRA_ID
export PIXIV_E2E_ILLUST_SEARCH_WORD PIXIV_E2E_DISCOVERY_WORD
export PIXIV_E2E_PROXY PIXIV_WEB_API_PROXY
export PIXIV_E2E_REAL_API=1
export PIXIV_E2E_WEB_API=1

go test ./e2e -count=1 -v
