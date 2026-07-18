# AI 协作文档

本目录承接 `AGENTS.md` 不适合长期展开的协作细则。根指令保持短而硬；需要专项规则时再读取这里或 `.agents/skills/`。

## 文档地图

- [Review checklist](review-checklist.md)：本仓库 code review 的边界、风险和测试清单。
- [Documentation guidelines](documentation-guidelines.md)：README、docs、ADR、CONTEXT、CHANGELOG、AGENTS 和 skills 的职责边界。

## 使用原则

- `AGENTS.md` 是跨 agent 的 canonical source of truth。
- `CLAUDE.md` 只包含 `@AGENTS.md`，不维护第二份规则。
- `.github/copilot-instructions.md` 是短提示，不复制主规则全文。
- `.agents/skills/` 只放高频、可复用、能减少误操作的项目技能。
