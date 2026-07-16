# Pixiv CLI 文档

## 项目定位

`pixiv-cli` 是一个 Go 版 Pixiv CLI、MCP stdio server 与 public `pixiv` SDK（`github.com/FlanChanXwO/pixiv-cli/pixiv`）。CLI/MCP 是 SDK consumer；调用方在自身 adapter 中定义采集、budget、filter 与持久化。无 refresh token 时，允许的匿名读操作可使用 Pixiv web/ajax API。

## 文档目录

- [架构说明](architecture.md)：入口、Pixiv/config/update/download 包边界、运行流程、Release asset 与信任约束。
- [开发流程](development.md)：本地环境、Rust staticlib、测试、构建、发布门禁、签名/tap 边界和 Git 注意事项。
- [MCP 工具](mcp-tools.md)：当前注册的 tools 与参数概览。
- [Go SDK 接口](sdk.md)：`*pixiv.Client` 的构造、模型、分页、错误和资源契约。
- [更新日志](../CHANGELOG.md)：按 Keep a Changelog 风格记录用户可感知变化。
- [AI 协作文档](agents/index.md)：agent 规则、review checklist 和文档职责边界。
- [ADR 0001](adr/0001-cli-thin-controller-and-bootstrap.md)：CLI thin controller、application services 与 bootstrap 分层决策。
- [ADR 0002](adr/0002-utils-and-common-boundaries.md)：`utils/*` 与 `common/constants` 的边界规则。
- [ADR 0003](adr/0003-agent-instruction-precedent-strategy.md)：AI agent 指令采用选择性先例而非单一模板的决策。
- [ADR 0004](adr/0004-auth-accounts-use-pixiv-uid.md)：本地 auth 账号使用 Pixiv UID 的决策。
- [ADR 0005](adr/0005-auth-login-real-browser-relay-without-ui-automation.md)：`auth login` 只接受显式 callback handoff、不读取浏览器凭据的决策。
- [ADR 0006](adr/0006-original-ugoira-resource-resolution.md)：认证会话下显式 Web ugoira original enrichment 的决策。
- [ADR 0007](adr/0007-platform-staticlibs-for-supported-source-builds.md)：提交平台 staticlib 以维持受支持源码构建的决策与 native evidence 约束。
- [ADR 0008](adr/0008-ed25519-signed-multi-channel-release-trust.md)：Ed25519 签名、多渠道更新和 tap 发布信任边界的决策。
- [ADR 0009](adr/0009-public-pixiv-sdk-and-caller-adapter.md)：public SDK、调用方 adapter 与无 HTTP Provider 的边界。
- [ADR 0010](adr/0010-http-client-timeout-and-context.md)：Pixiv HTTP client、显式注入与 context 生命周期策略。

## 快速命令

```bash
go test ./...
sh scripts/build.sh
go test ./scripts/platformsmokeworkflow -count=1
```

CLI 示例：

```bash
pixiv auth login
pixiv version --json
pixiv update --check
pixiv search "初音ミク" --json
pixiv download 123456
```

MCP 运行示例：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./build/pixiv mcp
```

真实 token 写在 inline 环境变量里也可能进入 shell history；长期使用建议通过 MCP client 的私密环境配置或本地账号管理。

stdout 保留给 MCP JSON-RPC；日志写入 stderr。
