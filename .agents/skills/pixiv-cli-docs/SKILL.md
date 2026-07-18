---
name: pixiv-cli-docs
description: Maintain pixiv-cli documentation; file responsibilities and routing live in docs/agents/documentation-guidelines.md.
---

# pixiv-cli Docs

新增、修改或审查本仓库文档。文件职责与路由表以 `docs/agents/documentation-guidelines.md` 为准；本文件只定义流程，不复制路由表。

## 流程

1. 读取 `docs/agents/documentation-guidelines.md`，确定内容应落在哪个文件。
2. 按目标文档的既有语言写作：英文文档用英文，简体中文文档用简体中文；命令、路径、包名和 code-id 保持英文。
3. 修改双语镜像时保持章节结构与行为语义对应；允许为自然表达调整句式，不得让两种语言出现不同契约。
4. 同一规则只写一处，其他位置用链接路由，不复制大段内容。
5. 完成后确认链接可从仓库内解析，检查双语结构/语义，并运行 `git diff --check`。

## 约束

- 不把长篇架构说明写回 `AGENTS.md`。
- 不为纯内部整理新增 changelog。
