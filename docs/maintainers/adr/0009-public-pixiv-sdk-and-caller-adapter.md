# ADR 0009：公开 Pixiv SDK 与调用方 adapter

## 状态

已采纳。

> 原状态：Accepted；**v1 已删除匿名 Web fallback**。本 ADR 决策清单中「内部使用物理包 `appapi`、`webapi`、`oauth`、`resource`」「匿名 Web read 仅在无 refresh token 时按配置 fallback」两条已被 v1 取代：`internal/services/pixiv/webapi` 与匿名 Web/AJAX 路径删除，无 refresh token 时不 fallback 而是返回认证要求/`removed_setting`；App API 出错不自动切换协议。v1 契约见 `AGENTS.md` 与 `docs/maintainers/architecture.md`；公开 SDK 契约本身（顶层 package、typed error、opaque cursor、ResourceRef/OpenResource/Download）继续有效。

## 背景

外部消费者需要 Pixiv 读取、写入、账号处理和安全媒体访问。早期替换讨论把它建模为 HTTP Provider，包含 discovery、source probe、RSS/crawler ingestion 与 capability negotiation。这些概念属于图库/采集调用方领域，不属于 CLI 仓库；加入它们会强制引入 service lifecycle、重复的持久化语义和宽而不稳定的接口。

## 决策

- 发布顶层 `pixiv` package（`github.com/FlanChanXwO/pixiv-cli/pixiv`），提供具体 `*pixiv.Client`、稳定模型、request/result、opaque cursor、typed error 和资源 API。
- 公开契约只通过 package 提供；不提供 HTTP server、`Discover`、Probe、Capabilities、RSS 或 crawler。
- `atri-setu-api` 等调用方拥有窄 adapter interface，以及自己的 source mode、budget、filter、cursor storage、persistence、scheduling 和 HTTP presentation。
- 内部使用物理包 `internal/services/pixiv/appapi`、`webapi`、`oauth`、`resource`；公开 SDK 负责路由和规范化，但不泄漏这些包。
- 有凭据时 App API 为主路径。匿名 Web read 仅在无 refresh token 时按配置 fallback；Web enrichment 必须显式，绝不隐藏 fallback。需要补全的公开操作遵循“完整结果或明确失败”的原子契约，具体取舍与未来 partial-result 变更门槛见 [ADR 0006](0006-original-ugoira-resource-resolution.md)。
- 媒体通过 policy 验证过的 `ResourceRef`、`OpenResource`、`Download` 提供，保持 stream ownership 与 SSRF controls。

## 后果

- CLI/MCP 和外部 Go 程序消费同一个公开契约，无需额外进程或 transport translation。
- 集成方可用小 mock 测试其 adapter，自选运行策略。
- 本仓库不承诺 collector-specific discovery 兼容性；调用方须将这类行为迁移到自己的领域层。
- 物理协议变更封装在顶层 `pixiv` 后，降低公开 API churn。
- 调用方可用 stable code、operation、backend 和 upstream status 区分主路径与补全失败，不需要猜测返回模型是否静默降级。
