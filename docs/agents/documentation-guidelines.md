# Documentation Guidelines

文档按读者和稳定性分层，避免把所有内容塞进根文件。

## 文件职责

- `README.md` / `README.zh-CN.md`：双语用户入口，只保留定位、安装、快速开始、接口选择、安全提醒和文档导航。
- `docs/cli-reference.md` / `docs/cli-reference.zh-CN.md`：双语 CLI 完整契约，包括命令、flag、账号认证、配置、环境变量、fallback 和更新行为。
- `CONTRIBUTING.md` / `CONTRIBUTING.zh-CN.md`：双语贡献流程、环境、TDD、架构边界和 PR checklist。
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

## 更新规则

- 影响项目定位、安装入口、快速开始或用户必须先知道的安全提醒时，同步双语 README；不要把精确接口表复制回入口页。
- CLI 命令、flag、账号、配置、环境变量、fallback、更新或输出契约变化时，同步双语 CLI reference。
- SDK API/模型/错误契约写 `docs/sdk.md`；MCP tool/schema/wire 语义写 `docs/mcp-tools.md`；入口页只链接这些权威文档。
- 内部边界变化优先写 `docs/architecture.md`；只有长期、难以逆转且未来维护者需要背景的取舍才写 ADR。
- 测试、构建与开发流程变化写 `docs/development.md`；贡献者入口级要求变化时同步双语 CONTRIBUTING。
- 术语冲突或领域概念变化才更新 `CONTEXT.md`。
- 纯内部重构、测试补充、文档清理通常不需要 changelog。
- 不为短期实现细节创建 ADR。
