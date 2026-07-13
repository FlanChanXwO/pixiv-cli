# 架构说明

## 入口与依赖方向

`cmd/pixiv` 只调用 `internal/cli`。CLI 与 MCP 是 adapter；公开 Go API 是 `pkg/pixiv`。生产对象只在 `internal/bootstrap` 组装。

```text
cmd/pixiv -> internal/cli -> internal/application -> pkg/pixiv -> internal/pixiv/{appapi,webapi,oauth,resource}
                                  ^                         ^
internal/mcpserver ---------------|                         |
internal/bootstrap ---------------+-------------------------+
```

- `internal/cli`：Cobra、flag、TTY、文本/JSON 输出、`auth login` 本地 UI adapter。业务调用经 `internal/application`，不直连内部 Pixiv transport。
- `internal/mcpserver`：tool 注册、MCP 输入输出适配；stdio runtime 在 `internal/bootstrap` 启动。
- `internal/application`：账号、配置、下载和 CLI/MCP 到公开 SDK 的应用编排；定义本地所需窄接口，避免公开 SDK 反向依赖 CLI/MCP。
- `pkg/pixiv`：稳定公开模型与具体 `*pixiv.Client`；不公开大而全 interface，不启动服务。
- `internal/pixiv/appapi`：认证 App API 主路径与写操作。
- `internal/pixiv/webapi`：匿名白名单读和明确 Web enrichment。
- `internal/pixiv/oauth`：refresh token 与 authorization-code exchange。
- `internal/pixiv/resource`：受策略限制的资源流传输。
- `internal/config` 与 `internal/storage/auth`：本地 `config.toml`、UID 账号与权限控制。
- `internal/download`：本地作品下载和 ugoira 处理。

旧 `internal/pixiv` facade 仅保留兼容入口；CLI/MCP 新领域能力经 application 使用 `pkg/pixiv`，不得直接依赖 `appapi` 或 `webapi`。

## SDK、配置与认证

`NewClient(Options)` 只使用显式选项，不读本地文件、不隐式认证。`OpenDefault(Options)` 使用 auth/config/环境选择账号，且每个公开操作取得新的配置快照；需要固定一个多请求操作时调用 `Snapshot(ctx)`。

OpenDefault 的 token 优先级：CLI 为 `--refresh-token` > `--uid` > `PIXIV_REFRESH_TOKEN` > 默认 UID；MCP 为 `PIXIV_REFRESH_TOKEN` > 默认 UID。`CurrentUserID` 只能从 OpenDefault 的认证快照取得，显式 access token 不臆测所属 UID。

## 上游路由

- 有 refresh token：App API 是主路径。App 认证、网络、服务端失败不自动回退 Web。
- 无 refresh token 且 `web_fallback_enabled=true`：仅匿名白名单读操作可走 Web API。
- `IllustDetail` 的 pages 补全与原始 ugoira resource metadata 是明确 enrichment；不是失败回退。
- 资源访问不需要 OAuth，但必须通过 `ResourceRef` policy 校验。

## 分页与资源

SDK `Cursor` 是版本化、不透明、绑定查询及 OpenDefault source 的 continuation。调用方只传回 `NextCursor`；CLI/MCP 把它适配为 `limit`/逻辑 `page`，不泄漏 token。

`ParseResourceRef` 只接受 Pixiv 官方资源域名，或调用方在 `ResourcePolicy` 显式批准的 mirror host/path prefix。`OpenResource` 仅转发 `Range`、`If-None-Match`、`If-Modified-Since`，过滤响应 header 并返回未预读流；调用方关闭 `Body`。`Download` 流式写入临时文件，成功后原子替换目标。

## 日志与错误

日志使用显式注入 `slog.Logger`；SDK 未注入时静默，不调用 `slog.Default`。CLI/MCP root logger 从 `log_level`/`log_format` 或 `PIXIV_LOG_LEVEL`/`PIXIV_LOG_FORMAT` 构造，输出 stderr。字段包括 component、operation、backend、duration、result、error_code、status 与已验证 ID，不记录 token、cookie、完整 URL、查询或 resource header。

SDK 以 `*pixiv.Error` 暴露 `Code`、`Operation`、`Backend`、`Retryable`、安全状态码和 ID；调用方用 `errors.As` 或 `errors.Is` 分支。MCP 失败使用 `isError=true`，不把失败伪装成空数据。

## 不在本仓库边界

本仓库不实现 HTTP API、Provider HTTP server、Discover/Probe/Capabilities、RSS、crawler、采集 source mode、预算、过滤、cursor 存储、数据库写入或图库调度。外部项目应围绕其领域定义窄 adapter；详见 [ADR 0007](adr/0007-public-pixiv-sdk-and-caller-adapter.md)。
