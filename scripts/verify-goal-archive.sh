#!/bin/sh
# 生成/校验 goal-3/ 与 goal-4/ 归档内容的 SHA-256 manifest。
# 用途：goal-3/、goal-4/ 是 untracked 独立归档（不纳入 codex/v1-sdk-rewrite 分支），
# 本 manifest 提供可复验的内容指纹：任何机器对同一归档运行本脚本应得到相同 manifest。
#
# 脚本放在代码工作树 scripts/ 下，但 goal-3/、goal-4/ 位于主 worktree 仓库根；
# 通过 GOAL_ROOT 指定（默认取主 worktree 根：脚本路径的 ../../..）。
#
# 用法（在主 worktree 仓库根执行）：
#   sh scripts/verify-goal-archive.sh --write  # 重新生成 manifest 文件
#   sh scripts/verify-goal-archive.sh          # 校验（默认，须与 manifest 一致）
set -eu

script_dir="$(cd "$(dirname "$0")" && pwd)"
goal_root="${GOAL_ROOT:-$(cd "$script_dir/.." && pwd)}"
manifest_file="$goal_root/goal-4/archive-manifest.sha256"

if [ ! -d "$goal_root/goal-3" ] || [ ! -d "$goal_root/goal-4" ]; then
    echo "goal dirs not found under $goal_root (set GOAL_ROOT to the main worktree root)" >&2
    exit 1
fi

if [ "${1:-}" = "--write" ]; then
    # 生成：相对路径 + SHA-256，排序稳定
    (
        cd "$goal_root"
        find goal-3 goal-4 -type f ! -name 'archive-manifest.sha256' -print0 \
            | sort -z \
            | xargs -0 shasum -a 256 \
            | sed "s|  \./|  |"
    ) > "$manifest_file"
    echo "wrote $manifest_file"
    exit 0
fi

if [ ! -f "$manifest_file" ]; then
    echo "manifest missing: $manifest_file (run with --write first)" >&2
    exit 1
fi

fail=0
(
    cd "$goal_root"
    find goal-3 goal-4 -type f ! -name 'archive-manifest.sha256' -print0 \
        | sort -z \
        | xargs -0 shasum -a 256 \
        | sed "s|  \./|  |"
) | diff - "$manifest_file" >/dev/null || fail=1

if [ "$fail" -eq 0 ]; then
    echo "goal archive manifest OK ($(wc -l < "$manifest_file") files)"
else
    echo "goal archive manifest MISMATCH (files changed since manifest was written)" >&2
    exit 1
fi
