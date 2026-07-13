# Pixiv CLI 文档

`pixiv-cli` 提供 CLI、MCP stdio 与公开 Go SDK。没有 HTTP service、Discover、RSS 或 crawler；需要采集编排的项目在自身边界实现 adapter。

- [SDK 接口](../pixiv-sdk-interface.md)：`pkg/pixiv` 的客户端、路由、cursor、资源和错误契约。
- [架构说明](architecture.md)：包职责、生产组装、路由与日志边界。
- [开发流程](development.md)：构建、测试、SDK 使用和配置。
- [MCP 工具](mcp-tools.md)：tools、参数、分页和 structured output。
- [更新日志](../CHANGELOG.md)：用户与集成方可见改动。
- [替换 PRD](../pixiv-integration-replacement-prd.md)：调用方 adapter 责任与替换范围。
- [AI 协作文档](agents/index.md)：agent 规则与文档职责。

## ADR

- [ADR 0001](adr/0001-cli-thin-controller-and-bootstrap.md)：CLI thin controller、application services、bootstrap 与 SDK 使用边界。
- [ADR 0002](adr/0002-utils-and-common-boundaries.md)：`utils/*` 与 `common/constants` 边界。
- [ADR 0003](adr/0003-agent-instruction-precedent-strategy.md)：agent 指令策略。
- [ADR 0004](adr/0004-auth-accounts-use-pixiv-uid.md)：本地账号使用 Pixiv UID。
- [ADR 0005](adr/0005-auth-login-real-browser-relay-without-ui-automation.md)：真实浏览器 OAuth 接力。
- [ADR 0006](adr/0006-original-ugoira-resource-resolution.md)：原始 ugoira 资源解析。
- [ADR 0007](adr/0007-public-pixiv-sdk-and-caller-adapter.md)：公开 SDK、调用方 adapter、无 HTTP/Discover 与物理拆包。
