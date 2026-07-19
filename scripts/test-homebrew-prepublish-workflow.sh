#!/bin/sh
# 验证只读 Homebrew 发布前演练 workflow 的安全结构；不访问 GitHub 或 Homebrew。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec go run "$repo_root/scripts/prepublishhomebrew" --workflow "$repo_root/.github/workflows/homebrew-prepublish-verify.yml"
