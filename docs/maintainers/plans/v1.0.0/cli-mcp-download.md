# v1.0.0 CLI、MCP、下载与内部边界

## CLI

Pixiv 根命令保持现有领域组织。内容命令没有有效本地账号时返回明确认证错误，不再匿名读取。
数据命令继续只使用 `pixiv auth use` 选择的账号或手工 `[account_pool]`，拒绝 `--uid`、
`--refresh-token` 并忽略 `PIXIV_REFRESH_TOKEN`。

新增 FANBOX 命令：

```text
pixiv fanbox auth import --stdin
pixiv fanbox auth import --from-browser BROWSER [--profile ID]
pixiv fanbox auth list
pixiv fanbox auth use UID
pixiv fanbox auth use --auto
pixiv fanbox auth remove UID
pixiv fanbox auth status [UID]

pixiv fanbox creators --kind supporting|following
pixiv fanbox posts SOURCE
pixiv fanbox tags CREATOR
pixiv fanbox home
pixiv fanbox supporting
pixiv fanbox download SOURCE...
pixiv fanbox mcp
```

Pixiv auth 同样提供 `auth use --auto`，用于删除显式 default key。

`--stdin` 与 `--from-browser` 互斥；Cookie 不允许作为位置参数或普通 flag。TTY 输入隐藏，
非 TTY 才读取 raw stdin。浏览器导入在保存前立即验证身份；失败不产生账号记录。

`SOURCE` 接受的 creator ID、tag、post ID、FANBOX URL 与旧 Pixiv FANBOX redirect 形成每个
命令的显式输入矩阵。无法唯一判断的字符串返回 `invalid_argument`，不猜测式 fallback。

CLI 文本、JSON、NDJSON、stderr 与 progress 都不得出现 refresh/access token、Cookie、签名
URL、browser path 或 profile 内容。FANBOX 不提供 session export；Pixiv auth export/bundle 的
现有 local-only secret stdout 契约保持不变。

## Credential data flow

Pixiv operation：

```text
config/default or account-pool selection
→ read refresh token from pixiv-cli.db
→ pixiv.Open
→ transactionally persist rotated Credentials
→ execute content operation with returned Client
```

rotation 持久化失败时内容请求不得开始。账号池只有在尚未提交 stdout record 或本地文件、且
收到带有效 `Retry-After` 的 typed 429 时才能切换账号。

FANBOX operation：

```text
config/default selection
→ read session from pixiv-cli.db
→ fanbox.Open
→ requested operation
```

`auth import` 额外执行 `ValidateSession`；普通 operation 遇到过期 session 返回
`credentials_expired`，不读取浏览器或自动切换账号。

## MCP

- `pixiv mcp` 只注册 Pixiv tools。
- `pixiv fanbox mcp` 只注册 FANBOX read/download tools。
- 两个 server 都不注册 auth、token、Cookie、browser 或 config tool。
- FANBOX server 默认使用 `[fanbox.auth].default_user_id`，未设置时使用首个入库账号；可在
  server 启动时使用非 secret UID 显式选择。
- Pixiv MCP 凭据选择遵循其独立 runtime 配置，不与 FANBOX 共享。
- stdout 只允许 JSON-RPC；runtime failure 保留 structured result 并设置 `isError=true`。
- 不创建项目级日志；错误和 structured output 使用相同脱敏边界。
- 两个 server 不共享 Client、credential、cursor、tool registry 或 fallback。

## 下载边界

公开 Pixiv/FANBOX Client 只持有 `OpenResource` 与单资源 `SaveResource`。高级 downloader 留在
内部并分两层：

```text
shared execution primitives
├── atomic destination
├── archive database primitives
├── sidecar writer
├── progress events
├── filename safety
└── resource transfer

product-specific source resolver
├── Pixiv artwork/page/ugoira expansion
└── FANBOX creator/post/asset expansion
```

共享层不认识 Pixiv/FANBOX token、领域模型或 endpoint。产品 resolver 不复制原子落盘、archive
或 progress 实现。第三方 embed 永不递归进入 downloader。

只有明确幂等的资源读取可依据有效 `Retry-After` 或调用方显式 policy 重试。不得继承当前
`RetryPolicy` 的固定默认次数/间隔，也不得因为输出较慢而设置固定失败 timeout。取消、断网、
状态码、challenge、资源身份变化和落盘失败都显露真实原因。

## Internal package 目标结构

```text
cmd/pixiv
internal/cli
internal/application/pixiv
internal/application/fanbox
internal/bootstrap
internal/mcpserver/pixiv
internal/mcpserver/fanbox
internal/services/pixiv/appapi
internal/services/pixiv/oauth
internal/services/pixiv/resource
internal/services/fanbox
internal/platform/browsercookies
internal/storage/authdb
internal/download
sdk
sdk/pixiv
sdk/fanbox
```

公开顶层目录统一为 `sdk/`。根 package `sdk` 只放协议无关基础契约；`sdk/pixiv` 与 `sdk/fanbox`
分别暴露产品 Client。旧顶层 `pixiv/` 在 v1 删除，且不创建顶层 `fanbox/`。

目标结构刻意不含 `internal/services/pixiv/webapi`。v1 实现时删除该目录及全部生产引用，不将其改名、
冻结或藏到 application 层；`resource` 只负责受控媒体读取，不是 Web API fallback。

`cmd/pixiv` 只委托 CLI；CLI 是 input/output adapter；application 编排账号、rotation、分页和
下载；bootstrap 是唯一生产 composition root。CLI/MCP 的产品能力只经对应 application service
调用对应 public `sdk/pixiv` 或 `sdk/fanbox` package，不直连 protocol adapter。

共享的只有协议无关的网络安全 primitive、分页遍历、错误、下载执行和测试基础设施。不得建立
通用 token、通用内容模型、通用 API client 或跨产品自动 fallback。

## Removed settings

`web_fallback_enabled`、`PIXIV_WEB_FALLBACK_ENABLED` 及其他 Web fallback 配置从 SDK、runtime、
文档与产品 Skill 删除。旧值显式存在时返回 `removed_setting`，不静默忽略。保留只用于迁移的
`pixiv config unset web_fallback_enabled`；删除后内容命令按正常认证规则运行。
完整理由和未来重新引入门槛见
[Web API 删除决策](web-api-removal.md)。
