---
name: pixiv-review
description: Review pixiv-mcp-server changes with repository-specific boundaries, MCP/CLI contracts, error handling, tests, and documentation requirements.
---

# Pixiv Review Skill

用于审查 `pixiv-mcp-server` 的本地改动。输出必须 finding-first：先列问题，按严重程度排序，再给简短总结。没有问题时明确说没有发现阻塞问题，并说明剩余测试风险。

## 必读上下文

- `AGENTS.md`
- `docs/agents/review-checklist.md`
- 相关改动附近代码和测试

## 审查重点

- `internal/cli` 是否回流业务逻辑，`internal/application` 是否承接用例，`internal/bootstrap` 是否仍是生产 wiring。
- CLI/MCP 是否通过 `internal/pixiv` facade 使用 Pixiv 能力。
- MCP tool 名称、参数、structured output、delivery mode、文本语义变化是否更新测试和 `docs/mcp-tools.md`。
- token 是否可能泄露；错误是否暴露真实原因；是否新增无依据限制或静默 fallback。
- 用户可见变化是否更新 README/docs，必要时更新 `CHANGELOG.md`。

## 输出格式

```text
Findings
- [P1] path:line 问题。影响。建议修复。

Open Questions
- ...

Summary
简短说明审查范围和剩余风险。
```

若无 finding：

```text
Findings
- 未发现阻塞问题。

Summary
说明已检查范围、已运行/未运行测试和剩余风险。
```
