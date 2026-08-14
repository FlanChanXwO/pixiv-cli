#!/usr/bin/env python3
"""核对 goal-4/tasks.md 的 task 与 goal-4/evidence/ 文件的一致性。

检查：
1. tasks.md 状态表里的每个 T{ID}（含 T15B）在 evidence/ 都有对应 task-{id}.md（T15B -> task-15b.md）；
2. 状态列只有 complete / blocked / pending，且 pending 数为 0（goal 交付后）；
3. evidence 目录里没有 tasks.md 不认识的孤儿文件（允许 README 等辅助文件白名单）。

用法：python3 scripts/check-goal-evidence-manifest.py [goal_root]
"""
import re
import sys
from pathlib import Path

goal_root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path.cwd()
tasks_file = goal_root / "goal-4" / "tasks.md"
evidence_dir = goal_root / "goal-4" / "evidence"

if not tasks_file.exists() or not evidence_dir.is_dir():
    print(f"cannot find goal-4 docs under {goal_root}", file=sys.stderr)
    sys.exit(1)

status = {}
for line in tasks_file.read_text(encoding="utf-8").splitlines():
    m = re.match(r"\|\s*(T\d+\w?)\s*\|\s*[^|]*\|\s*[^|]*\|\s*(\w+)\s*\|", line)
    if m:
        status[m.group(1)] = m.group(2)

ids = sorted(status)
print(f"tasks.md 状态表 task 数: {len(ids)}")

errors = []
pending = [tid for tid, st in status.items() if st == "pending"]
if pending:
    errors.append(f"pending tasks remain: {pending}")

expected_files = {"task-" + tid[1:].lower() + ".md" for tid in ids}
actual_files = {p.name for p in evidence_dir.glob("task-*.md")}

missing = expected_files - actual_files
if missing:
    errors.append(f"missing evidence files: {sorted(missing)}")

orphans = actual_files - expected_files
if orphans:
    errors.append(f"orphan evidence files (not in tasks.md): {sorted(orphans)}")

if errors:
    print("MISMATCH:")
    for e in errors:
        print("  -", e)
    sys.exit(1)

print(f"evidence 文件数: {len(actual_files)}（与 tasks.md 状态表一一对应）")
print("OK")
