# Pixiv 接入替换 PRD

## 目标

让调用方以 Go package 或现有 CLI/MCP 接入 Pixiv，不引入 HTTP service。公开实现是顶层 `pixiv`（`github.com/FlanChanXwO/pixiv-cli/pixiv`）的具体 `*pixiv.Client`；调用方可以替换其自身 adapter，但不需要、也不应依赖本仓库提供的泛化 Provider。

目标调用方包括 `atri-setu-api`：它可继续拥有 source mode、采集预算、过滤、cursor 持久化、数据库入库、图片代理和任务调度，只把 Pixiv 上游读取、认证、规范化模型与安全资源读取委托给 SDK。

## 非目标

- 不提供 HTTP API、HTTP provider server 或服务发现。
- 不提供 `Discover`、Probe、Capabilities、RSS、crawler、feed 或 PID 抓取。
- 不替换调用方数据库、审核、随机选图、管理后台、预算、过滤、持久化 cursor 或采集编排。
- 不引入外部 Pixiv SDK 或新运行时依赖。
- 不新增 CLI 子命令；现有 CLI/MCP 是 SDK 的 adapter。

## 用户故事

1. 作为 Go 调用方，我可以以 `NewClient` 提供显式 access token，或以 `OpenDefault` 使用本地账号和配置。
2. 作为调用方，我可以查询作品、用户、收藏、关注、推荐、排行榜、标签、ugoira 数据，并获取稳定的规范化模型。
3. 作为调用方，我可以用不透明 cursor 安全续页，而无需理解 App/Web 的 offset 或 page。
4. 作为调用方，我可以在自己的 adapter 中定义最小接口，并以 mock 验证业务，不需运行真实 Pixiv。
5. 作为图片代理实现者，我可以解析可信 `ResourceRef` 并流式打开资源，同时保留 range/条件请求与 SSRF 边界。
6. 作为运维者，我可以在 stderr 日志中看到可分类错误、后端、操作与耗时，而不会泄露 token、cookie、URL 或查询内容。
7. 作为 CLI/MCP 使用者，我可以按 `limit`/逻辑 `page` 列表、查看当前用户作品/收藏/关注，并执行收藏/关注写操作。

## 公开契约

### 客户端与模型

- `NewClient(pixiv.Options)`：纯显式构造；不读本地文件、不刷新 token、不发网络请求。
- `OpenDefault(pixiv.Options)`：读取本地 auth/config 和环境；每个公开操作创建一次 operation snapshot。
- `Snapshot(ctx)`：调用方明确要求多个相关请求复用同一 snapshot。
- 公开 request/result/model 与 `*pixiv.Error` 都放在顶层 `pixiv`；不泄漏上游 DTO、CLI/MCP 类型或 internal 包。
- 不预置公共大 interface；调用方按用例定义窄接口，再以 `*pixiv.Client` 适配。

### 路由与认证

- 有 refresh token：App API 为主路径；App 认证、网络、服务端失败不自动 fallback Web。
- 无 refresh token 且 `web_fallback_enabled=true`：仅匿名白名单读能力可使用 Web API。
- Web pages、原始 ugoira metadata 等为明确 enrichment；不是 App 失败回退。
- `OpenDefault` token 选择遵循 CLI/MCP 的既有优先级；每操作重新读取配置，避免显式 Reload API。

### 分页

- SDK 用版本化 opaque `Cursor`，与操作、查询和 OpenDefault source 绑定；空 cursor 表示无下一批。
- 调用方只存储和回传 SDK 输出的 cursor，不解析、不合成、不把它暴露给终端用户。
- CLI 与 MCP 把 cursor 适配为 `limit`、逻辑 `page`；无 `limit` 保持一个上游批次兼容行为，`limit=0` 表示完整遍历。
- CLI `--offset` 与 MCP 旧 continuation 参数只为兼容保留，标记 deprecated。

### 资源

- `ParseResourceRef` 必须先验证资源 URL，默认允许 Pixiv 官方资源，额外镜像必须在 `ResourcePolicy` 指定 exact host 和 path prefix。
- `OpenResource` 允许 range、ETag、修改时间条件请求，返回白名单 header 与未预读的 `Body`。调用方负责关闭 body、处理 client disconnect 和下游写入失败。
- `Download` 流式写同目录临时文件，成功后原子替换目标；不把完整资源读入内存。
- 此契约支持调用方图片代理，禁止把它作为任意 URL fetch；重定向也必须重新验证，防 SSRF。

### 错误与日志

- `*pixiv.Error` 提供 `Code`、`Operation`、`Backend`、`Retryable`、HTTP status 与已验证 ID；调用方用 `errors.As`/`errors.Is` 分支。
- 不可用作品、未认证、无权限、限流、上游格式错误、上游不可用、参数错误必须保留可区分语义；不得伪装成空数据。
- SDK 仅用调用方显式注入的 `slog.Logger`；为空时静默。CLI/MCP logger 使用 `log_level`/`log_format` 或对应环境变量，输出 stderr。

## 调用方 adapter 示例

`atri-setu-api` 可拥有如下本地边界（示意，非 SDK 新接口）：

```go
type PixivReader interface {
    SearchIllust(context.Context, pixiv.SearchIllustRequest) (*pixiv.IllustListResult, error)
    IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error)
}

type SourceRun struct {
    Mode   string
    Budget Budget
    Cursor string // 调用方存 SDK opaque cursor
    Filter Filter
}
```

adapter 根据 `Mode` 选择 SDK 方法，将 `NextCursor` 存回自己的状态；它负责 budget、filter、筛选后入库、重试策略和生命周期。SDK 不知道 `SourceRun`，也不提供 `Discover`。

## 验收标准

- `github.com/FlanChanXwO/pixiv-cli/pixiv` 可由外部 Go module 直接导入，构造 `*pixiv.Client`，不需 HTTP server。
- 外部调用方可以用其 own adapter mock SDK 的窄方法集。
- CLI 可列出用户作品、收藏、关注；`USER_ID` 可选并解析为当前认证用户。
- MCP 的 SDK 路径同步暴露对应用户读取与写操作；这些错误为 `isError=true` 并保留 structured/text，列表使用 `page`/`limit` structured pagination。遗留 tool 保持既有文本结果兼容，不承诺统一 `isError`。
- SDK 和 CLI/MCP 均不暴露 cursor token、不把 App 失败静默改走 Web。
- 图片代理可使用 `ResourceRef`/`OpenResource` 流式工作，非法 URL、header、redirect 与写入错误明确失败。
- README、接口说明、架构、ADR、MCP 文档、开发文档、CHANGELOG 与知识图谱同步。
