# Documentation Guidelines

文档按读者和稳定性分层，避免把所有内容塞进根文件。

## 文件职责

- `README.md`：用户入口，描述 CLI/MCP 用法、配置、环境变量和主要行为。
- `docs/index.md`：文档导航。
- `docs/architecture.md`：包职责、运行流程和已知约束。
- `docs/development.md`：本地环境、测试、构建、Git、changelog 和开发流程。
- `docs/mcp-tools.md`：MCP tools 的名称、参数和返回语义。
- `CONTEXT.md`：Pixiv/MCP 领域词汇与关系；不要放通用工程流程。
- `docs/adr/`：难以逆转、未来会疑惑、经过真实取舍的决策。
- `CHANGELOG.md`：用户和集成方可感知的变化。
- `AGENTS.md`：agent 的短主规则和路由。
- `.github/copilot-instructions.md`：Copilot 短提示。
- `.agents/skills/`：高频 agent 工作流。
- `.understand-anything/`、`docs/.understand-anything/`：入库的 understand-anything 知识图谱快照（代码图谱与文档图谱），是 agent 快速认识项目的首选入口；结构性变更后重新生成并随改动提交。

## 更新规则

- 功能、API、配置、CLI/MCP 行为、输出语义、测试流程或限制变化时，同步 README 或 docs。
- 公开行为变化优先写 `README.md`；内部边界变化优先写 `docs/architecture.md` 或 ADR。
- 术语冲突或领域概念变化才更新 `CONTEXT.md`。
- 纯内部重构、测试补充、文档清理通常不需要 changelog。
- 不为短期实现细节创建 ADR；只有真实取舍且未来维护者需要背景时才新增。
