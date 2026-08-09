#!/usr/bin/env bash
# 运行显式授权的本机 SDK E2E；凭据只由测试进程从 authdb/Keychain 读取。
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  scripts/test-e2e.sh [--pixiv-only | --fanbox-only | --fanbox-post-only]

The default runs both current SDK E2E tests. Credentials are never accepted as
arguments or environment variables: Pixiv reads the selected local authdb
account and FANBOX reads the documented macOS Keychain item.

The real FANBOX test additionally requires these non-secret target values:
  FANBOX_E2E_CREATOR_ID, FANBOX_E2E_TAG, FANBOX_E2E_POST_ID,
  FANBOX_E2E_POST_URL

The post-only FANBOX test requires only FANBOX_E2E_POST_ID and
FANBOX_E2E_POST_URL; it verifies post.info and permits zero file assets.

Optional:
  PIXIV_E2E_PROXY   Explicit non-secret proxy URI for the SDK test process.
  FANBOX_E2E_SOLVER_URL    Explicit FlareSolverr service URL for challenge recovery.
  FANBOX_E2E_SOLVER_PROXY  Independent upstream proxy URI used by FlareSolverr.
EOF
}

mode=all
while (( $# > 0 )); do
	case "$1" in
	--pixiv-only)
		if [[ "$mode" != all ]]; then
			usage >&2
			exit 2
		fi
		mode=pixiv
		;;
	--fanbox-only)
		if [[ "$mode" != all ]]; then
			usage >&2
			exit 2
		fi
		mode=fanbox
		;;
	--fanbox-post-only)
		if [[ "$mode" != all ]]; then
			usage >&2
			exit 2
		fi
		mode=fanbox-post
		;;
	--help|-h)
		usage
		exit 0
		;;
	*)
		usage >&2
		exit 2
		;;
	esac
	shift
done

case "$mode" in
all)
	PIXIV_SDK_E2E=1 FANBOX_SDK_E2E=1 go test ./e2e -run '^TestReal(Pixiv|Fanbox)SDKRead$' -count=1 -v
	;;
pixiv)
	PIXIV_SDK_E2E=1 go test ./e2e -run '^TestRealPixivSDKRead$' -count=1 -v
	;;
fanbox)
	FANBOX_SDK_E2E=1 go test ./e2e -run '^TestRealFanboxSDKRead$' -count=1 -v
	;;
fanbox-post)
	FANBOX_SDK_E2E=1 FANBOX_E2E_POST_ONLY=1 go test ./e2e -run '^TestRealFanboxSDKPostInfo$' -count=1 -v
	;;
esac
