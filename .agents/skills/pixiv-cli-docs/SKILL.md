---
name: pixiv-cli-docs
description: Maintain pixiv-cli documentation; locale and maintainer routing live in docs/maintainers/agents/documentation-guidelines.md.
---

# pixiv-cli Docs

新增、修改或审查本仓库文档。文件职责与路由表以 `docs/maintainers/agents/documentation-guidelines.md` 为准；本文件只定义流程，不复制路由表。

## 流程

1. 读取 `docs/maintainers/agents/documentation-guidelines.md`，确定内容应落在哪个 locale 或 maintainer 文件。
2. 按目标 locale 写作；命令、路径、包名和 code-id 保持英文。
3. 修改已翻译的 public contract 时保持行为语义对应；允许自然调整句式，不得让不同语言出现不同契约。
4. 同一规则只写一处，其他位置用链接路由，不复制大段内容。
5. 完成后确认链接可从仓库内解析，检查 locale 导航与语义，并运行 `git diff --check`。

## 约束

- 不把长篇架构说明写回 `AGENTS.md`。
- 不为纯内部整理新增 changelog。
