#!/bin/sh
# 以 YAML 结构验证发布 workflow 的不可退让安全与质量约束；无需 GitHub token 或 Actions runner。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow="$repo_root/.github/workflows/release.yml"

exec go run "$repo_root/scripts/releaseworkflow" --workflow "$workflow"
