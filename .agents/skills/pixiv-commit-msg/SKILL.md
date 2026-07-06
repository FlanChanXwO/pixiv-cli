---
name: pixiv-commit-msg
description: Generate a one-line commit message for pixiv-cli from staged changes and recent commit style.
---

# Pixiv Commit Message Skill

根据暂存区生成一行提交信息。默认只看 staged changes；用户明确要求包含未暂存内容时才看全部 diff。

## 必须读取

```bash
git status --short
git diff --cached --stat
git diff --cached
git log --oneline -10
```

若暂存区为空，不要编造提交信息，直接说明没有 staged changes。

## 风格

- 一行输出，不加解释、项目符号或代码块。
- 优先贴近仓库近期风格，通常使用 Conventional Commits：
  - `feat: ...`
  - `fix: ...`
  - `docs: ...`
  - `refactor: ...`
  - `test: ...`
  - `chore: ...`
- subject 使用英文、小写开头，除非专有名词需要大写。
- 控制在 72 字符左右；不要写 `misc`、`update files`、`wip`。

## 判断

- 文档和 agent 文件：优先 `docs:`
- 包边界或内部结构：优先 `refactor:`
- 行为修复：优先 `fix:`
- 测试补充：优先 `test:`
- 构建、脚本、依赖、工程杂项：优先 `chore:`
