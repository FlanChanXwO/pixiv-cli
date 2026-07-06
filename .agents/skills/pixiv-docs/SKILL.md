---
name: pixiv-docs
description: Maintain pixiv-cli documentation without bloating root agent instructions; route README, docs, ADR, CONTEXT, CHANGELOG, AGENTS, Copilot hints, and skills correctly.
---

# Pixiv Docs Skill

用于新增、修改或审查本仓库文档。正文默认简体中文，命令、路径、包名和 code-id 保持英文。

## 必读上下文

- `AGENTS.md`
- `docs/agents/documentation-guidelines.md`
- 当前要修改的目标文档

## 路由规则

- 用户使用和公开行为：`README.md`
- 文档导航：`docs/index.md`
- 包职责和流程：`docs/architecture.md`
- 开发、测试、构建和 changelog 流程：`docs/development.md`
- MCP tool 参数和返回语义：`docs/mcp-tools.md`
- 难以逆转且有取舍背景的决策：`docs/adr/`
- 领域词汇：`CONTEXT.md`
- 用户可感知变化：`CHANGELOG.md` 的 `[Unreleased]`
- Agent 主规则：`AGENTS.md`
- Copilot 短提示：`.github/copilot-instructions.md`
- 高频 agent 工作流：`.agents/skills/`

## 写作约束

- 不把长篇架构说明塞回 `AGENTS.md`。
- 不在多个文件复制同一大段规则；用链接和路由。
- 不为普通内部整理新增 changelog。
- 不把通用工程流程写进 `CONTEXT.md`。
- 链接要能从仓库内解析；完成后运行 `git diff --check`。
