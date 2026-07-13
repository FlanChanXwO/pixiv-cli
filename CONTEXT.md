# Context

## Domain

本项目是 Pixiv CLI、MCP stdio server 与 Go SDK。公开能力在 `pkg/pixiv`，入口是具体 `*pixiv.Client`；不是 HTTP 服务，也不是图库/采集系统。

## Vocabulary

- **SDK client**：`*pixiv.Client`，调用方直接构造或 `OpenDefault` 打开。
- **Caller adapter**：外部项目拥有的窄接口和业务适配层。它决定 source mode、budget、filters、cursor 持久化、入库与调度。
- **Operation snapshot**：`OpenDefault` 每个公开操作读取一次配置、账号与 OAuth 结果得到的稳定 client；`Snapshot(ctx)` 让调用方显式复用。
- **Opaque cursor**：SDK 生成、绑定操作/查询/source 的版本化 continuation；调用方不可解析，CLI/MCP 不暴露。
- **Explicit enrichment**：为完整数据显式调用 Web pages 或 ugoira metadata；不代表 App API 失败回退。
- **ResourceRef**：经 policy 验证的图片/zip URL；只能用 `ParseResourceRef` 创建后给 `OpenResource`/`Download`。

## Relationships

- CLI/MCP 是 SDK consumer，不是 SDK owner；生产对象由 `internal/bootstrap` 组装。
- `atri-setu-api` 等调用方可持有自己的 adapter；本项目不定义其 `Discover`、Probe、Capabilities 或 source 领域。
- App API 在有 token 时是主路径；匿名 Web API 仅在无 token 且配置允许时服务白名单读操作。
- 资源代理可以流式转发 `OpenResource` 的 body，但调用方必须关闭它，且不得绕过 `ResourcePolicy`。

## Boundary rules

- 不新增 HTTP server、RSS、crawler、Discover/Probe/Capabilities 或泛化 Provider interface。
- 不在日志中写 refresh token、cookie、完整 URL、用户搜索词或 resource headers。
- SDK 错误以 `*pixiv.Error`、`errors.As`/`errors.Is` 处理；不要把上游失败当空列表。
- MCP stdout 只能是 JSON-RPC；日志和诊断写 stderr。
