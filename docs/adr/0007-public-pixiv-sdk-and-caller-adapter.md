# ADR 0007：公开 Pixiv SDK 与调用方 adapter

## 状态

已采纳。

## 背景

外部消费者需要 Pixiv 读取、写入、账号处理和安全媒体访问。早期替换讨论把它建模为 HTTP Provider，包含 discovery、source probe、RSS/crawler ingestion 与 capability negotiation。这些概念属于图库/采集调用方领域，不属于 CLI 仓库；加入它们会强制引入 service lifecycle、重复的持久化语义和宽而不稳定的接口。

## 决策

- 发布 `pkg/pixiv`，提供具体 `*pixiv.Client`、稳定模型、request/result、opaque cursor、typed error 和资源 API。
- 公开契约只通过 package 提供；不提供 HTTP server、`Discover`、Probe、Capabilities、RSS 或 crawler。
- `atri-setu-api` 等调用方拥有窄 adapter interface，以及自己的 source mode、budget、filter、cursor storage、persistence、scheduling 和 HTTP presentation。
- 内部使用物理包 `internal/pixiv/appapi`、`webapi`、`oauth`、`resource`；公开 SDK 负责路由和规范化，但不泄漏这些包。
- 有凭据时 App API 为主路径。匿名 Web read 仅在无 refresh token 时按配置 fallback；Web enrichment 必须显式，绝不隐藏 fallback。
- 媒体通过 policy 验证过的 `ResourceRef`、`OpenResource`、`Download` 提供，保持 stream ownership 与 SSRF controls。

## 后果

- CLI/MCP 和外部 Go 程序消费同一个公开契约，无需额外进程或 transport translation。
- 集成方可用小 mock 测试其 adapter，自选运行策略。
- 本仓库不承诺 collector-specific discovery 兼容性；调用方须将这类行为迁移到自己的领域层。
- 物理协议变更封装在 `pkg/pixiv` 后，降低公开 API churn。
