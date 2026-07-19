---
name: pixiv-cli-commit-message
description: Generate a one-line Conventional Commits message for pixiv-cli from staged changes.
---

# pixiv-cli Commit Message

根据暂存区生成一行提交信息。默认只看 staged changes；暂存区为空时直接说明，不编造。

## 读取

```bash
git status --short
git diff --cached
git log --oneline -10
```

## 风格

- 一行输出，不加解释、项目符号或代码块。
- Conventional Commits，贴近仓库近期风格：`feat` / `fix` / `docs` / `refactor` / `test` / `chore`。
- subject 英文、小写开头，约 72 字符；不写 `misc`、`update files`、`wip`。
- type 判断：行为修复 `fix`；包边界或内部结构 `refactor`；文档和 agent 文件 `docs`；测试补充 `test`；构建/脚本/依赖 `chore`。
