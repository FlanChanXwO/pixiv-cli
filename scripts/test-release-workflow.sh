#!/bin/sh
# 静态验证发布 workflow 的不可退让安全与产物约束；无需 GitHub token 或 Actions runner。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow="$repo_root/.github/workflows/release.yml"

fail() {
	printf '%s\n' "release workflow test: $*" >&2
	exit 1
}

[ -f "$workflow" ] || fail "missing $workflow"

# 每一个 action 均必须锁到完整 Git object ID，避免 tag 被移动后改变发布供应链。
if rg -n '^\s*uses:' "$workflow" | rg -v '@[0-9a-f]{40}(\s*(#.*)?)$' >/dev/null; then
	fail 'every uses: reference must be pinned to a full 40-character SHA'
fi

for runner in macos-15-intel macos-15 ubuntu-24.04 ubuntu-24.04-arm windows-2025 windows-11-arm; do
	rg -F -- "runner: $runner" "$workflow" >/dev/null || fail "missing native runner $runner"
done

for required in \
	'permissions: {}' \
	'contents: read' \
	'contents: write' \
	'environment: release' \
	'DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}' \
	'needs: validate' \
	'go run ./scripts/releaseassets validate' \
	'cargo test --locked --offline' \
	'go test ./...' \
	'go vet ./...' \
	'go run ./scripts/licensebundle --check' \
	'sh scripts/test-package-release.sh' \
	'sh scripts/test-release-workflow.sh' \
	'--repo-root .' \
	'--target' \
	'--input-dir dist' \
	'--output-dir release' \
	'--private-key "$key_path"' \
	'checksums.json' \
	'gh release create' \
	'--draft' \
	'--verify-tag' \
	'--prerelease' \
	'gh release view' \
	'gh release edit' \
	'--draft=false' \
	'fetch-depth: 0' \
	'persist-credentials: false' \
	'git merge-base --is-ancestor' \
	'origin/$DEFAULT_BRANCH' \
	'RELEASE_SIGNING_PRIVATE_KEY' \
	'umask 077'; do
	rg -F -- "$required" "$workflow" >/dev/null || fail "missing required release gate: $required"
done

if rg -n 'pull_request_target|pull_request:' "$workflow" >/dev/null; then
	fail 'release workflow must not run on pull request events'
fi

rg -n '^\s+tags:' "$workflow" >/dev/null || fail 'release workflow must be tag-triggered'
rg -F -- "'v[0-9]*'" "$workflow" >/dev/null || fail 'release workflow tag filter must require a v-prefixed SemVer tag'

# 私钥只能出现在已验证 tag commit 属于默认发布分支之后的 publish 步骤中。
ancestry_line=$(rg -n 'git merge-base --is-ancestor' "$workflow" | sed -n '1s/:.*//p')
secret_line=$(rg -n 'RELEASE_SIGNING_PRIVATE_KEY' "$workflow" | sed -n '1s/:.*//p')
[ -n "$ancestry_line" ] || fail 'missing trusted-source ancestry gate'
[ -n "$secret_line" ] || fail 'missing signing secret declaration'
[ "$ancestry_line" -lt "$secret_line" ] || fail 'signing secret is declared before trusted-source ancestry gate'
