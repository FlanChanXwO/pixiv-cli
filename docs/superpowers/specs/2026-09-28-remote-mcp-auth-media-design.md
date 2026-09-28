# Remote MCP、鉴权与媒体交付设计

日期：2026-09-28

核验基线：`97caf95fdbe7758e9bd0f1038162f7bfd1d11fc5`。本文是修订后的实现合同，不表示尚未实现的 remote MCP 已通过云端 host 验收。实际执行结果见文末。

## 目标

将 pixiv-cli 的 MCP 从本地 stdio 模式重构为面向自托管远程连接器的单一 MCP 服务，使 ChatGPT、Gemini 与 Claude 能通过同一个公开 HTTPS 地址连接，并完整使用 Pixiv 与 FANBOX 能力。

本次是明确的 breaking change：

- 只保留 Streamable HTTP，不保留 stdio、旧版独立 SSE transport 或 transport 兼容层；不禁止 Streamable HTTP 协议自身使用的 SSE 响应编码。
- Pixiv 与 FANBOX 合并到一个 MCP endpoint。
- Pixiv tools 全部增加 `pixiv_` 前缀；FANBOX tools 保持 `fanbox_` 前缀。
- MCP 不再提供“下载到 MCP server 本地磁盘”的 download tools；媒体通过 MCP 实际交付给 host。
- 内置单用户 OAuth 2.1 Authorization Server，通过 DCR + Authorization Code + PKCE 供 ChatGPT、Gemini、Claude 连接。

“全自动授权”必须分清三条链路：

| 链路 | 用户动作 | 自动完成部分 |
| --- | --- | --- |
| Connector 连接本实例 | 添加 MCP URL，首次验证 owner 身份并确认授权 | DCR、回调、code exchange、后续 refresh；不用复制 client secret/token/code |
| 使用服务器已有 Pixiv 账号 | 通常无需动作；需要时选择账号 | 读取现有账号库、刷新并持久化凭证、后续业务请求 |
| 新增 Pixiv 账号 | 在浏览器完成 Pixiv 要求的登录/确认；浏览器设备需要现有 pixiv-cli helper | helper 自动回传 callback，服务器交换并保存凭证，页面确认成功；不用把 callback 粘贴到聊天 |

这里的自动化不是跳过用户同意、验证码或上游账号验证。服务器已有账号的日常使用不要求安装桌面 helper；新增账号的免复制回调路线明确依赖它。这替代旧稿“没有 helper 时粘贴 callback 也算自动登录通过”的验收方式，不承诺任意纯浏览器/手机环境都能拦截 Pixiv 的固定回调。

## 非目标

- 不做多用户、组织、角色或权限矩阵。
- 不做 read/write/admin OAuth scopes；tool 调用审批交给 host。
- 不为某一家 host 写 transport 或 OAuth 特判。
- 不保留旧 tool name alias。
- 不实现旧版独立 SSE transport 或 stdio fallback。
- 不让 MCP Apps 成为核心功能依赖。
- 不把 CLI 的本地下载能力删除；`pixiv download` 继续负责本机下载与归档。
- 不为 Gemini 当前未向任意第三方 MCP 开放的 MCP Apps UI 做规避或伪装。

## 当前状态与需要移除的假设

当前仓库：

- 使用 `github.com/modelcontextprotocol/go-sdk v0.8.0`。
- `pixiv mcp` 与 `pixiv fanbox mcp` 启动两个独立 stdio server。
- Pixiv MCP tool 名没有统一 `pixiv_` 前缀；FANBOX 已使用 `fanbox_` 前缀。
- Pixiv `download` / `download_random_from_recommendation` 写 MCP server 本地文件，仅返回 path / `file://` / MIME / size。
- `fanbox_open_resource` 只读取 resource 状态和 metadata，不返回内容 bytes。
- `reverse_search` 接受 server 本地文件路径或 HTTP(S) URL；remote MCP 继续保留这些经过 owner 授权的输入能力，并明确其 server-side 语义。

Remote MCP 后，server 本地路径不能再充当对云端 host 的媒体交付结果；它作为反搜输入仍有效。用户上传的本地图片也不等于 server 本地路径。

## 总体架构

```text
ChatGPT ─┐
Gemini  ─┼─ OAuth 2.1 / DCR / PKCE ─────────────┐
Claude  ─┘                                      │
                                                ▼
                                      pixiv-cli HTTP host
                                                │
                                          POST /mcp
                                                │
                                      Streamable HTTP only
                                                │
                                      one mcp.Server
                                        ┌───────┴───────┐
                                        ▼               ▼
                                  Pixiv runtime     FANBOX runtime
                                        │               │
                                    sdk/pixiv       sdk/fanbox
```

只有一个 MCP protocol server。Pixiv 与 FANBOX 继续保留独立 runtime、SDK client、credential domain 与业务 package；合并发生在 composition root 和 tool registry，不把两个产品的业务状态混在一起。

`pixiv mcp` 在没有子命令时直接启动长期 HTTP 服务。`pixiv fanbox mcp` 删除。

公共部署要求：

- pixiv-cli 只负责 HTTP 应用服务，不内置证书申请或 ACME。
- 配置一个 listen address 与一个 canonical base URL。
- pixiv-cli 本身不强制 TLS。本地 Agent / Inspector 可以直接连接 `http://127.0.0.1:<port>/mcp` 或其他明确的本地 HTTP 地址。
- 本实例的 OAuth issuer、metadata/endpoint URL、MCP resource URL 从配置的 canonical base URL 构造，不信任请求的 `Host` / `X-Forwarded-*`；返回 host 的 OAuth redirect_uri 则来自已注册且精确校验过的 client metadata，不能错误地改成本实例域名。
- 当服务需要给 ChatGPT/Gemini/Claude 这类云端 connector 使用时，canonical base URL 配置为公网 HTTPS 地址，并由 Caddy、nginx、Cloudflare 或其他现有反向代理负责 TLS。
- 三家云端 connector 使用同一个公开 `https://.../mcp`；本地 Agent 不需要为了使用同一 Streamable HTTP server 额外部署 HTTPS。

## MCP SDK 与协议

升级到当前稳定的 `github.com/modelcontextprotocol/go-sdk` v1 系列；设计验证基线为 2026-09-28 的 v1.8.0。

Transport 只有 Streamable HTTP。协议 revision 使用官方 SDK 的标准协商能力，不自行实现旧协议兼容层，也不因某个 host 维护独立 transport。

使用 `mcp.NewStreamableHTTPHandler`，明确设置 `StreamableHTTPOptions.Stateless=true`。官方 SDK v1.8.0 的 `2026-07-28` revision 要求 stateless；旧 revision 的标准协商交给同一 SDK。不要自己实现 `initialize`/`server/discover` 或额外维护 MCP session store。只有 OAuth grant 和业务账号状态需要持久化。

Bearer middleware 覆盖 `/mcp` 的每次请求，再交给 SDK 处理方法、版本、取消和响应形式。Origin 校验使用 SDK 能力并与显式部署 origin 对齐，不从任意 Host 转发头推导信任域。涉及账号的 tool/resource/result 不启用 public HTTP cache；不能因 stateless 或 SDK 支持 cache 而跨账号缓存。

验收标准是 ChatGPT 与 Gemini 能连接同一个 Streamable HTTP endpoint；Claude 按相同 MCP/OAuth 标准实现，但不作为本次 live E2E 阻塞项。

## Unified tool namespace

统一命名：

```text
pixiv_search_illust
pixiv_illust_detail
pixiv_illust_ranking
pixiv_reverse_search
pixiv_add_bookmark
pixiv_follow_user
...

fanbox_home
fanbox_creator
fanbox_post
fanbox_open_resource
...
```

规则：

- 所有现有 Pixiv tools 改为 `pixiv_*`。
- 所有 FANBOX tools 保持 `fanbox_*`。
- 不注册旧名称 alias。
- `reverse_search` 改名 `pixiv_reverse_search`。
- 旧 MCP `download` 与 `download_random_from_recommendation` 删除，不改名为任何 `pixiv_*_download` tool。
- CLI `pixiv download` 保留。

## OAuth 2.1：单 owner、内置 Authorization Server

采用内置极简单用户 OAuth 2.1 server，不依赖外部 IdP。

### Identity model

实例只有一个 owner：

```text
owner
  ├─ ChatGPT OAuth client/grant
  ├─ Gemini OAuth client/grant
  └─ Claude OAuth client/grant
```

多个 connector grant 只代表同一个 owner，不形成多用户系统。

不实现：

- username/password account system
- email / registration / password recovery
- roles / organizations / tenants
- per-client Pixiv account state
- 本次不新增 FANBOX account login/switch MCP tools；FANBOX 继续使用现有本地 account/session 选择规则

### Owner secret

本地执行：

```text
pixiv mcp auth init
```

生成高熵随机 owner secret。只展示给实例管理员一次；server 仅保存其 SHA-256 verifier。由于 secret 是机器生成的高熵随机值，不接受用户自定义弱密码，因此无需引入密码哈希依赖。

如需重新初始化，使用同一命令的显式 `--reset`：

```text
pixiv mcp auth init --reset
```

普通 `init` 遇到已初始化状态不覆盖、不重新泄露 secret。`--reset` 重新生成 owner secret，并使现有 OAuth grant/token、owner 浏览器会话及待交换 code 失效；保留 DCR client metadata 和 MCP selected account，避免无谓要求用户删除、重建 connector。第一版不增加独立 reset 子命令。

OAuth authorize 页面要求用户输入 owner secret。该 secret：

- 不进入 MCP tool arguments。
- 不提供给 LLM。
- 不作为 OAuth bearer token。
- 不等同于 Pixiv/FANBOX credential。

授权页使用标准密码输入，支持浏览器密码管理器。身份验证后可以复用当前浏览器的 HttpOnly、SameSite=Lax 会话 cookie；公网 HTTPS 使用 Secure。本机 HTTP 不因 Secure cookie 而无法登录。会话仅存内存，绑定 owner 当前 verifier；重启/reset 后重新验证即可，不新增 cookie 数据库。

即使浏览器已验证 owner，首次授权一个 DCR client 仍显示 client 名称、redirect origin 和“允许/拒绝”；client 提供的名称仅作未验证标签。POST 使用独立 CSRF nonce，不能把 OAuth client 的 state 当 CSRF 校验，也不能让任意注册的 client 借 owner cookie 静默获得权限。完成确认后直接 302 返回 host，不让模型中转授权码。

### OAuth endpoints

公共 HTTP surface：

```text
GET  /.well-known/oauth-protected-resource
GET  /.well-known/oauth-protected-resource/mcp
GET  /.well-known/oauth-authorization-server
POST /oauth/register
GET  /oauth/authorize
POST /oauth/authorize
POST /oauth/token
ANY  /mcp  (由 SDK 决定支持的方法，不代表所有方法都成功)
```

要求：

- DCR。
- Authorization Code flow。
- PKCE S256。
- `resource` 参数按 MCP authorization 规范传播并验证。
- redirect URI 必须与注册 metadata 精确匹配。
- authorization code 单次使用。
- DCR 第一版注册 public client，使用 `token_endpoint_auth_method=none`，不签发或保存 client secret；PKCE S256 是 authorization code exchange 的必需保护。
- access/refresh token 为高熵 opaque token，server 只保存不可逆 hash。
- refresh token 每次成功 exchange 后旋转：旧 refresh token 立即失效，并签发新的 access/refresh token pair。
- `/mcp` 每次请求验证 bearer token；未授权返回 HTTP 401 + 正确的 `WWW-Authenticate`，不伪装成 MCP tool error。

第一版选择目前 ChatGPT 仍支持的 DCR public client，不实现 CIMD，也不写 `if ChatGPT` / `if Gemini` / `if Claude` 分支。最新版 MCP 已将 DCR 标为兼容路径、推荐 CIMD，因此“DCR 可行”必须由目标 host 实测，而不是宣称 DCR 是永远唯一的标准。若真实 host 不接受此注册方式，属于第一条集成切片的阻塞，不能等整套媒体/UI 做完后才发现。

OAuth scope 第一版只有：

```text
mcp
```

不构建 `mcp:read` / `mcp:write` / `mcp:admin`。

### 最小可互操作合同

- Protected resource metadata 的 `resource` 是完整 canonical MCP URL，例如 `https://example.test/mcp`；`authorization_servers` 指向 canonical issuer；`WWW-Authenticate` 的 `resource_metadata` 指向实际存在的 metadata。上面的根路径和 resource-specific 路径共用同一 handler，不是两套实现。部署带 path prefix 时按规范构造 well-known 路径并测试，不能直接错误地拼接 issuer。
- AS metadata 明确发布 `authorization_endpoint`、`token_endpoint`、`registration_endpoint`、`response_types_supported=["code"]`、`grant_types_supported=["authorization_code","refresh_token"]`、`code_challenge_methods_supported=["S256"]`、`token_endpoint_auth_methods_supported=["none"]`、`scopes_supported=["mcp"]`。
- DCR 返回 201 和稳定的 client_id；省略 auth method 时选择 none，显式要求不支持的方法则返回标准注册错误，不能签发假的 client_secret。redirect URI 不带 fragment；拒绝 javascript/data 等执行性 scheme；接受合规 HTTPS、本机 loopback HTTP 和 native app 私有 scheme，authorize 时仍与注册值精确匹配。注册过程不抓取任意 client logo/metadata URL。
- authorize 只接受 code + S256 + 注册过的 redirect URI。将 client_id、redirect_uri、resource、scope、challenge 和 owner generation 绑定进服务端 code 记录；原样返回 state。只有实际返回标准 `iss` 参数时才声明 `authorization_response_iss_parameter_supported=true`。
- token 接收标准 form body；验证 code、client、redirect、PKCE 和 resource，再签发 Bearer。错误是 OAuth HTTP JSON，而非 MCP result。响应带 `Cache-Control: no-store`；token 不进 query、access log 或 Gallery。
- authorization code 从签发起有效 10 分钟，access token 有效 1 小时并返回 `expires_in=3600`，分别依据 RFC 6749 §4.1.2 和 RFC 6750 §5.3；它们不是浏览器登录/工具调用的硬超时。测试注入 clock，不真的等待一小时。refresh grant 不增加无依据的闲置过期阈值，持续到撤销/reset。
- refresh 绑定同一 client/resource/scope，不扩大权限。事务内旋转并保留已使用 refresh hash，命中已使用值时按 RFC 9700 撤销对应 grant；随机无效值不得撤销别人的 grant。并发 refresh 不可同时成功，断线后重放不能伪装成功；需要重新授权时明确报告。
- code/token 消费、refresh 旋转、grant 撤销必须在成功持久化后才发布成功响应。磁盘错误不得返回一个重启后无效的“成功 token”。过期 access token 在新请求处返回 401，不据此中断已经授权、正在执行的长媒体请求。

## Tool 权限与 host 审批

权限等级由 ChatGPT/Gemini/Claude host 控制，不由 OAuth scope 重复实现。

每个 tool 必须准确设置标准 annotations，例如：

- 纯读取：`readOnlyHint=true`。
- 可逆写入（收藏、关注、账号切换）：`readOnlyHint=false`, `destructiveHint=false`。
- 删除/撤销类操作：`readOnlyHint=false`, `destructiveHint=true`。
- 重复执行安全的操作按真实语义设置 `idempotentHint=true`。
- `openWorldHint` 按 tool 是否访问外部网络真实标注。

保留“只读 / 可逆写入 / 删除撤销”三类行为标注及 host 的逐工具审批能力。它们是 hints，不是 server 强制的三档授权，也不能保证各家 UI 使用相同的三级开关。真正的 tool allow/deny 仍由 host 设置；本版不引入第二套 scope/RBAC。

账号 list/status/use 是本地账号操作，`openWorldHint` 按实际调用链设置；status 读取状态不应每次联网 refresh。反搜会向外部 provider 上传图片，必须在描述中说明，不能用 readOnlyHint 掩盖此行为。Annotations 不替代 server 的认证和输入校验。

## Pixiv MCP 当前账号

MCP 有一个 owner 级别、跨 connector 共享的当前 Pixiv 账号：

```text
MCP owner
  └─ selected_pixiv_user_id
       ↑
  ChatGPT / Gemini / Claude shared
```

工具：

```text
pixiv_account_list
pixiv_account_status
pixiv_account_use
pixiv_account_login_start
```

语义：

- 选择结果持久化在 `mcp-state.json` 的 `selected_pixiv_user_id`，不新增 SQLite state/table，不写入 CLI 的 `[pixiv.auth].default_user_id`。
- ChatGPT 调 `pixiv_account_use(B)` 后，Gemini 与 Claude 后续调用也使用 B。
- 显式 MCP current account 覆盖 read pool 自动轮换；MCP 的读取和写入都使用该账号，避免用户切换后仍被 pool 偷偷换号。
- 如果 selected account 被删除、过期或不可用，不静默切到其他账号；`pixiv_account_status` 返回明确状态。
- 成功登录新账号且当前没有可用 selected account 时，将新账号设为 MCP current account；已有可用 current account 时不隐式切换。

首次初始化还没有 selection 时，优先采用现有 CLI 显式 default；没有 default 且只有一个本地账号时采用该账号；有多个且无 default 时返回 `selection_required` 和可选账号，不随便挑第一个。已设置但失效的 selection 不触发这条首次初始化规则。

`account_use(user_id)` 先检查本地账号存在，再持久化选择并返回；并发调用按持久化提交顺序生效。每次业务调用进入 runtime 时固定其 account snapshot，切换只影响后续调用，不改变已在执行的收藏/媒体操作。SDK 刷新继续使用现有 credential revision/CAS 持久化；不在 MCP 复制 OAuth token 管理。

`account_status` 返回 `selected_user_id`、本地凭证状态和 selection 状态；本地有 token 不等于实时上游有效。正常业务沿用现有 SDK 鉴权与错误分类，不为每个 tool 额外请求一次 CurrentUser；上游确认失效时明确返回 `credentials_expired`。它还接受可选 `login_id` 查询新增账号流程状态，不再增加一个重复的 login-status tool。

## Pixiv 新账号：复用现有自动 callback handoff

不再暴露接受 raw callback 的 `pixiv_account_login_complete`。继续使用 `sdk/pixiv.LoginSession` 与 `internal/services/pixiv/account.LoginService`，凭证只经现有账号服务落盘，MCP 不接受 raw refresh token。

```text
pixiv_account_login_start
  -> login_id + 本实例的 relay session URL
  -> 用户打开页面，点击使用本机 Pixiv 登录助手
  -> 已安装 helper 领取本次 authorization URL 并打开 Pixiv
  -> Pixiv 登录成功，浏览器触发固定 pixiv:// callback
  -> helper 直接 POST callback + handoff proof 到本实例
  -> 原 LoginSession 进行 PKCE exchange
  -> LoginService 保存账号；必要时更新 MCP selection
  -> 浏览器显示最终成功，account_status(login_id) 返回 completed
```

### 已有实现与最小复用边界

现有 `internal/cli/commands/pixiv/auth/login_pair_relay.go` 已实现 `/session/{id}`、`/start/{id}`、`/callback/{id}`、`/result/{id}` 的一次性 handoff；`loginhelper` 已有 macOS、Windows、Linux 的 handler。复用这些 wire format、proof 校验、成功通知和清理规则，不另做浏览器插件、远程桌面、Playwright/Chromium 登录服务或上传平台。

当前 `WaitForHandoffRelayLoginCode` 自己开 listener，不能原样在每次 MCP tool 中调用。将其 HTTP handler/session 部分及紧密依赖的纯页面/URL helper 移至 `internal/services/pixiv/account/loginrelay`；CLI 保留浏览器/终端/独立 listener 包装，MCP 把同一 handler 挂在主 mux 的 `/pixiv-login/` 前缀。OS handler 仍留在原位置。不得让 service/MCP 反向导入 CLI command，也不为每次登录创建新端口。

私有 callback HTTP 入口以 handoff proof + 该 session 的 PKCE 绑定认证，不要求桌面 helper 拿到 MCP bearer。`login_id` 不是 callback 授权凭证，proof 不放 MCP result、日志或错误。继续拒绝跨 origin 重定向及未 start/已消费 callback；`pixiv://` 正式回调可不带 state，不能添加与现有 SDK 冲突的要求。

### Tool 和生命周期

- `login_start` 返回 `login_id`、`authorization_url`（本实例 relay 页面，不是含 Pixiv callback 的 URL）、`status=waiting_for_user`、`requires_local_helper=true`、简短使用说明。
- 当前 helper 只保存一个 active handoff，所以本实例只保留一个活动 Pixiv login；重复 start 返回同一 pending session，不悄悄覆盖。需要重新开始时显式传 `restart=true`，清理旧 session 后再创建，不实现队列/多 worker。
- `account_status(login_id)` 返回 `waiting_for_user / exchanging / completed / failed / not_found`，成功时包含账号摘要。只保留当前/最近一次流程结果，开始下一次替换；不积累无界历史。
- 请求结束不等于取消浏览器登录：pending session 归 server lifecycle，而不是 start tool 的短 request context。用户显式 restart、server shutdown 或确定的 callback/exchange/persistence 错误负责结束流程；不加“等待 30 秒后失败”或固定轮询次数。
- 成功页必须晚于账号持久化。账号已入库但 selection 写失败时返回明确失败及 `account_saved=true, selection_updated=false`，不得虚构事务回滚或要求用户再次登录；修复写入问题后用 `account_use` 即可。
- helper 缺失/不支持的平台显示安装与重开步骤，不能靠浏览器定时器猜测是否安装。桌面安装沿用已有官方安装器/handler 初始化；不为 remote server 安装浏览器。用户手动粘贴 callback 不能让这一验收项变成通过。

“无本机 helper + host 不提供 callback 捕获能力 + Pixiv 固定回调 + 不复制 callback”不能同时作为已验证承诺。本版选择已有 helper 这条最小可实现路线；已有服务器账号仍然零 helper 使用。

## 媒体交付

### 原则

Search/detail tools 保持轻量：

```text
pixiv_search_*
pixiv_illust_detail
pixiv_illust_ranking
pixiv_illust_recommended
pixiv_illust_related
...
```

只返回 structured metadata / refs，不隐式下载所有图片。

实际媒体通过专用只读 tool：

```text
pixiv_artwork_media
```

MCP 媒体职责是“把作品内容交给 host”，不是“写到 MCP server 磁盘”。

### Static artwork

默认：

- `quality=regular`
- 请求一个完整作品时默认所有 pages。
- 支持明确 page selection。
- 用户明确要求原图时才使用 `original`。

输入确定为 `illust_id`、可选 `pages`（从 1 开始的页码数组，缺省代表全部）、`quality=thumbnail|regular|original`、`animation_format=gif|apng`。thumbnail 仅供 Gallery 按需请求，不改变模型直接调用时的 regular 默认值；优先使用 SDK 已有的缩略图资源，不在本版引入图片缩放依赖。空 pages、越界页码、动画不适用的静态参数返回明确输入错误；重复页码去重且保留请求顺序。

返回同时包含：

1. structured metadata：illust ID、title、page、MIME、size 等。
2. MCP `ImageContent`：真实图片 bytes，使 ChatGPT/Gemini/Claude 可以实际展示或交给本地 agent 保存。

多图作品必须可以取得全部图片。不能静默只返回第一页；若 host/transport 的真实限制阻止一次完整传输，应返回明确失败/未完成状态，并允许按页再次读取，不做无提示截断。

structured result 明确包含 `total_pages`、`requested_pages`、`delivered_pages`、`failures`、`complete`，每页 metadata 与 content index 对应。可以部分成功时保留成功内容并标明未交付页面；整条响应被 host 拒绝不能记录为部分成功或已交付。已知 host 实际 payload 限制应通过按页调用满足，不发明统一页数/字节截断常量。metadata-first discovery + 明确 pages 的 Gallery 调用避免每次预览下载整个作品。

图片读取走账号绑定的 SDK Resource/现有媒体链路，保留所需 Pixiv Referer 等请求语义；不能把 pximg URL 直接交给 iframe 后假设一定能加载。临时文件只作为既有 encoder/snapshot 的内部实现，用完清理，不成为公开 MCP download contract。

### Ugoira

复用现有 Ugoira pipeline 与 Rust encoder：

- 默认 GIF。
- APNG 可选。
- 不新增 ffmpeg runtime dependency。
- 返回实际动画媒体内容，而不是只返回 Pixiv URL 或 server local path。

Host 是否内联播放动画属于 host 展示能力；MCP 必须至少实际交付动画文件/内容，而不是只报告“已找到”。

静态图片使用标准 `ImageContent`。GIF/APNG 与非图片附件通过标准 embedded resource 的 blob 内容交付，保留真实 MIME、文件名、字节数；动画同时提供静态预览，不把其首帧冒充完整动画。不假定所有 host 把 `image/gif` ImageContent 当可播放文件。必须先做真实 host 的二进制交付验证：协议能序列化不等于聊天可展示/下载；不支持时如实阻塞对应验收，不在实现末尾才发现。

### 删除 server-side MCP download

MCP 不再有：

```text
download
download_random_from_recommendation
pixiv_artwork_download
```

本地 agent 如果需要下载文件，可把 `pixiv_artwork_media` 返回的媒体保存到自己的文件系统；CLI 用户继续使用 `pixiv download`。

## FANBOX 媒体

统一 remote MCP 后，`fanbox_open_resource` 不再只返回 status/Content-Type/Content-Length。

它应成为真正的 resource 内容读取能力：

- 图片返回实际 image content。
- 非图片文件返回实际 binary/resource content，并同时保留安全 structured metadata。
- 不暴露 MCP server 本地路径。
- FANBOX post/list/detail 仍保持 metadata-first，不在每次列表读取时无脑下载全部媒体。

Pixiv 与 FANBOX 的媒体实现可以复用窄的 MCP content conversion helper，但不能合并两个产品的 SDK/runtime ownership。

## MCP Apps Gallery

MCP Apps Gallery 第一版纳入实现，并作为 server 固有能力始终声明，但只能作为 progressive enhancement。第一版不增加 `mcp_apps_enabled`、`gallery_enabled` 等配置或 feature flag；不支持 MCP Apps 的 host 直接忽略 UI extension/resource，继续使用 structured result 与媒体 fallback。只有未来出现可复现的 host 兼容性故障时，才评估是否需要禁用开关。

UI resource 使用标准 MCP Apps，不依赖 ChatGPT 私有 UI API。视觉 discovery tools 直接关联同一个 `ui://pixiv-cli/gallery` resource（使用当前 MCP Apps 标准的 tool metadata），不额外增加只为“渲染”而存在的公开 tool。Gallery 通过标准 app bridge 调用已有 MCP tools。

实现使用 `_meta.ui.resourceUri` 及 `text/html;profile=mcp-app` resource MIME，按标准完成 app bridge 初始化。仅增加一个内嵌 gallery HTML/JS 资源，使用已有工具链或原生 DOM；不新增 React/Vite/npm 构建系统。使用官方 bridge contract 的必要消息，不做通用 RPC 框架。

Gallery 接收当前 discovery result，按可见卡片调用同一个 `pixiv_artwork_media(pages=[1], quality="thumbnail")`，详情再请求用户选择的页码；从实际 bytes 创建 image/blob URL，离开视图时释放。不用服务器本地路径，也不依赖第三方图片跨域或 cookie。分页沿用 discovery cursor，不按固定卡片数量截断结果；展示 loading、empty、error、partial、bookmark 状态，异步结果必须绑定本次视图，避免旧搜索覆盖新搜索。

标题、作者、标签作为文本渲染，不拼进 HTML。Gallery 不接收 bearer、Pixiv token、FANBOX cookie；CSP 仅声明实际需要的资源/连接域。图库授权由 host bridge 传递，iframe 不自行保存 OAuth 凭证。下载/播放按钮必须实际走完目标 host 的文件能力，不以本地浏览器能创建 Blob 就推断 ChatGPT/Gemini 一定支持。

Gallery 面向视觉 discovery：

- search results
- ranking
- recommended
- related
- artwork detail / multi-page view
- bookmark state
- Ugoira indicator / playback where host supports it

支持 MCP Apps 的 host 可显示 Gallery；不支持时，同一个 tool 的 structured result 与 `pixiv_artwork_media` 必须保证完整功能。

核心不允许出现：

```text
supports_gallery == false
→ 只能返回一个图片 URL
```

正确 fallback：

```text
structured result
→ model selects artwork
→ pixiv_artwork_media
→ ImageContent / actual media
```

验收范围：

- ChatGPT：实际验收 MCP Apps Gallery。
- Gemini：当前对任意自定义第三方 MCP 不开放可靠 MCP Apps UI，因此不验收 Gallery；必须验收无 Gallery fallback。
- Claude：按标准 MCP Apps contract 实现，但不作为本次 live 验收阻塞项。

不为 Gemini trusted-MCP 限制实现任何绕过。

## Reverse image search input

`pixiv_reverse_search` 继续支持 MCP server 本地文件，并新增 remote-host 需要的输入方式。所有本地文件语义都明确指向 **运行 pixiv-cli MCP server 的机器**，而不是 ChatGPT/Gemini/Claude 所在设备。

支持以下输入路径：

### 1. ChatGPT uploaded file

使用 ChatGPT 官方 MCP 扩展：

```text
_meta["openai/fileParams"] = ["image"]
```

`image` 必须是顶层 object，并精确声明 `download_url`、`file_id`、`mime_type`、`file_name` 四个字段；前两项 required，后两项 optional。不能仅声明 download URL，也不能把字符串路径塞进同一个 file object schema。

保留 `source` string 作为 local path / file URI / HTTP(S) URL 输入；`source` 与 `image` 恰好提供一个，handler 验证后统一交给现有 snapshot/facade。保留 provider 的既有枚举与默认规则。`file_id` 只作为 host 描述信息，不能当下载地址；临时 URL 失效返回明确错误，让用户重新附加文件，不重试猜测其它 URL。不得把临时 URL、文件 ID 或原始路径回写到普通结果/日志。

这是 ChatGPT 增强，不改变标准 MCP server contract，也不增加 OpenAI SDK dependency；Go MCP SDK 的 tool `_meta` 可直接承载该 metadata。

### 2. Local path / file://

保留现有本地文件能力：

- 常规本地文件路径。
- `file://` URI。
- 文件必须由 MCP server 进程可读，并继续复用现有 regular-file / snapshot 校验与清理逻辑。
- 这些路径永远解释为 MCP server 所在机器的文件系统，不假装是 connector 客户端的本地文件。

不增加 `allow_local_file_access` feature flag。该能力是经过 OAuth 授权的单 owner MCP 的一部分，并在文档中明确其 server-side filesystem 语义。

### 3. HTTP(S) URL

保留通用 HTTP(S) URL 输入，包括 localhost、私网、loopback/link-local 等现有能力；不额外为 remote MCP 增加 public-network-only guard。该设计延续当前 trusted-owner 语义：被授权的 connector 可以要求 pixiv-cli 读取 server 可访问的 URL。

ChatGPT `openai/fileParams` 的临时 `download_url` 也按普通 HTTP(S) source 读取，不引入厂商专用网络策略。

### Gemini / other host fallback

第一版不实现自建 browser-upload session、上传页面或临时文件托管服务。对于无法把聊天附件直接传给第三方 MCP 的 host，`pixiv_reverse_search` 仍可使用：

- MCP server 本地路径 / `file://`（本地 Agent 或 server-side 文件）。
- HTTP(S) image URL。

ChatGPT 额外获得 `openai/fileParams` 的直接附件输入。若未来 Gemini 或其他 host 的真实使用证明 URL / local-file 路径不足，再单独设计上传 fallback；本版本不预埋。

## 安全边界

- pixiv-cli 不在应用层全局强制 HTTPS；本地 loopback/private deployment 可以使用 HTTP。
- ChatGPT/Gemini/Claude 云端 connector 的公开入口使用公网 HTTPS；TLS 可由现有反向代理终止。
- OAuth authorize/token/register 与 `/mcp` 共用配置的 canonical origin。
- canonical origin 只来自显式配置，不从反向代理请求头推断。
- DCR redirect URI 精确匹配。
- PKCE 只允许 S256。
- authorization code 一次性。
- owner secret、OAuth access/refresh token 使用高熵随机值；持久化只存 verifier/hash。
- MCP/Pixiv/FANBOX credential、PKCE verifier、provider API keys 不进入 tool structured output、日志或 Gallery payload。
- `pixiv_reverse_search` 可读取 server 本地 regular file、`file://` URI 与 server 可访问的 HTTP(S) URL；这是单 owner 授权后的显式能力，文档必须说明它访问的是 server 侧资源。
- OAuth 认证只证明 MCP owner；Pixiv/FANBOX credential domain 继续独立。

## 错误语义

HTTP/Auth boundary：

- 未授权 MCP request → HTTP 401 + `WWW-Authenticate`。
- OAuth metadata / DCR / token errors → 标准 OAuth HTTP error。
- 这些错误不能伪装成 tool result。

Tool boundary：

- 业务输入错误、Pixiv/FANBOX upstream 错误、media fetch/encode 错误继续使用 MCP error result + safe structured output。
- 不把 token、raw callback secret、local temp path 放入错误文本。
- 多图或 provider 部分成功时保留已完成结果，并显式标记 partial/failures；不静默丢数据。

## 配置与持久化

**本次不新增 SQLite database、table 或 migration。** 现有 `pixiv-cli.db` 继续只承担既有 Pixiv/FANBOX account storage，不为 MCP OAuth 扩表。

MCP 只新增一个私有状态文件，例如：

```text
<app-data>/mcp-state.json
```

复用仓库现有 private-file 原子写入能力，不新增存储依赖。该文件只保存必须跨进程重启存在的少量状态：

- owner secret verifier。
- DCR client 的最小 metadata 与 client ID。
- OAuth grant/token verifier/hash。
- shared `selected_pixiv_user_id`。

以下状态只放内存，不持久化：

- authorization code。
- Pixiv `pixiv_account_login_start` 的 one-shot login session。
- owner 浏览器会话和授权表单 CSRF nonce。

Streamable HTTP 使用 stateless，不新增 MCP session state。内存 code/owner session 绑定持久 owner generation，reset 后不能继续换取 token。

因此 server 重启可以中断尚未完成的授权/登录流程，但不会丢失已连接 connector 的注册/grant 或 MCP 当前 Pixiv 账号；这是单 owner 自托管场景可接受的最小状态模型。

config 只保存服务启动所需的非 secret 配置：

- MCP listen address。
- canonical/public base URL（本地可为 HTTP，云端 connector 部署时为 HTTPS）。
- 现有 proxy / reverse-search provider 配置继续沿用既有 owner。

不新增泛化 IAM、session store、KV abstraction、额外数据库或 schema migration。

### 状态一致性与配置合同

持久数据采用一个具体结构：`version`、`owner_verifier`、`clients`、`grants`、`selected_pixiv_user_id`。grant 保存 client/resource/scope、active token hashes/expiry、已使用 refresh hashes/revoked 状态；不保存原始 token。不要为这些字段分别建 repository interface 或多个 JSON 文件。

复用 `internal/storage/file/lock.WithPrivateLock` 在 `.lock` 侧车锁内执行“读取最新文件 → 验证 → 修改 → `internal/storage/file/atomic.AtomicWrite`”；侧车只用于已有文件锁协调，不是另一份状态存储。不要对旧的进程内副本 blind overwrite。Bearer 校验/账号读取从原子文件取得当前快照，初版不增加 cache invalidation、文件 watcher 或后台同步。

因此 `auth init --reset` 可以与服务进程协调，新请求立即读取撤销后的状态；已经开始的工具请求不强制中断。文件损坏、版本不支持、权限错误都显式失败，绝不能自动创建空状态或默认放行。多个 mutation 并发、refresh 与 reset 并发、磁盘写失败均有回归。临时授权/login state 仍限定单个服务实例，不承诺多副本负载均衡。

配置落在 `[mcp]`：`listen_addr`、`base_url`，并提供对应 `pixiv mcp --listen-addr ... --base-url ...` 启动覆写，遵循仓库已有配置优先级。两项首次启动均须通过配置或参数明确提供，缺失时显示可直接修改的本地/公网启动示例；不新增环境变量解析器，不自动探测公网地址，也不为持久 OAuth grant 默默分配每次重启不同的端口。端口号是部署选择，不是业务硬限制。

启动前校验配置及 owner 状态；尚未 init 时明确提示先运行 `pixiv mcp auth init`，不能匿名启动远程私有工具。输出可复制 MCP endpoint 与初始化状态，但不自动打印 owner secret。绑定失败直接报告地址错误，不静默换端口，避免已有 connector 的 issuer/resource 失效。

## 迁移与删除

实现时明确删除：

- stdio transport runner 与相关 SIGPIPE/MCP stdio wiring。
- `pixiv fanbox mcp` command。
- 两个独立 `mcp.NewServer` composition roots，收敛为一个 unified server。
- Pixiv 旧 tool names。
- MCP `download` / `download_random_from_recommendation`。
- remote MCP 中 server local path / `file://` 作为媒体交付 contract。
- 旧 `reverse_search` tool 名；server-local-file / `file://` 输入能力保留在新的 `pixiv_reverse_search`。

不保留 deprecated alias、compat flags 或 fallback transport。

MCP OAuth/state 实现不得新增 SQLite migration 或 table；若实现计划出现数据库 schema 变更，应视为偏离本设计并重新审查。

## 验收

### Automated

至少覆盖：

- Streamable HTTP server lifecycle。
- protected-resource metadata 与 authorization-server metadata。
- DCR client registration validation。
- Authorization Code + PKCE S256。
- one-time code consumption。
- bearer/refresh token hash lookup、refresh rotation 与 owner-reset invalidation。
- owner secret verification。
- unified Pixiv + FANBOX tool discovery。
- 所有 Pixiv tool name 都有 `pixiv_`，所有 FANBOX tool name 都有 `fanbox_`。
- tool annotations 与写操作分类。
- shared MCP selected Pixiv account persistence。
- selected account unavailable 时不静默换号。
- `pixiv_artwork_media` 单图、多图、regular/original、GIF/APNG。
- `fanbox_open_resource` 实际 media/binary content。
- ChatGPT fileParams metadata schema。
- reverse-search local path / `file://` / HTTP(S) source 与现有 snapshot 安全输出行为不回归。
- reverse-search local path / file:// / HTTP(S) 与 ChatGPT fileParams 输入。
- Gallery tool/resource contract 在无 UI host 时仍保持完整 structured/media fallback。

授权 code/access token 的期限、refresh reuse 撤销和单个活动 Pixiv handoff 是本次明确的约束：前两者来自 OAuth 规范，后者来自已有 desktop helper 的单 active-handoff 结构；它们不限制正常工具输出长度、不截断页面、不在用户等待登录时强制失败。为相应触发条件、恢复方式与成功路径补回归，不能扩展成通用重试/限流框架。

### ChatGPT live E2E — blocking

必须实测：

1. 通过 custom third-party remote MCP 添加 `/mcp`。
2. DCR + OAuth owner authorization 完成。
3. tool discovery 正常，旧名不存在。
4. Pixiv account list/use/status 可用且重连后选择保持。
5. 已有账号直接复用；新增账号通过真实浏览器 + 已安装 helper 完成自动 callback，并以账号持久化和后续读取为成功证据。复制 callback 到聊天不算通过。
6. 搜索/detail 不隐式拉所有图片。
7. MCP Apps Gallery 能显示 Pixiv 搜索/作品预览。
8. `pixiv_artwork_media` 能把单图与多图实际发到聊天。
9. Ugoira 至少以实际 GIF/APNG 内容交付，而不是 URL。
10. 用户上传图片可通过 `openai/fileParams` 调用 `pixiv_reverse_search`。

### Gemini live E2E — blocking for core, not Gallery

必须实测：

1. Gemini custom Connected App 能通过 DCR/OAuth 连接同一个 `/mcp`。
2. 普通 Pixiv/FANBOX tools 正常。
3. tool annotations 不阻止正常 read/write approval flow。
4. `pixiv_artwork_media` 在没有 MCP Apps Gallery 的前提下实际交付图片。
5. 多图作品可完整取得。
6. reverse search 可通过 HTTP(S) URL 完成；本地 Agent 可继续使用 local path / file://。

不以 Gemini MCP Apps Gallery 作为验收项。

### Claude

按同一 Streamable HTTP、DCR/OAuth、tool/media、MCP Apps 标准实现；不要求本次 release 前进行 live E2E，不作为 merge/release blocker。

### 不能互相替代的验收证据

| 证据 | 能证明 | 不能证明 |
| --- | --- | --- |
| 现有 relay fixture | handler/proof/状态/清理合同 | 真实浏览器已安装 helper、真实 Pixiv 发回 callback |
| 本地账号 refresh + SDK read | 已有凭证有效、业务读取链路可用 | 新账号登录、MCP AS、ChatGPT/Gemini 连接 |
| SDK/Inspector 对新 HTTP server 的 OAuth 测试 | HTTP/注册/PKCE/refresh 的实现合同 | 云端 host 实际走完整授权流程 |
| ChatGPT/Gemini 产品内真实连接 | 该 host 的注册、回调、token、tool 调用 | 另一个 host、其它终端/浏览器也通过 |
| Blob/ImageContent 字节校验 | MCP 确实返回完整内容 | 聊天 UI 已经展示、下载或播放文件 |

每项 live 记录 commit、host 产品、操作系统/浏览器、实际步骤、通过/失败/未运行及脱敏错误类别。HTTP trace 只记录方法、端点名、状态码、相关 request ID；不记录 query、Authorization、Cookie、code、signed URL 或 response body。不能把 Gemini CLI/API 当 Gemini 产品 Connected App 的验收替身。

## 实施顺序与文件责任

本次交付可在同一实现分支连续完成，不拆成未经确认的新产品阶段；但先验证高风险外部边界，不能用“最后再验收”掩盖 host 不支持。所有行为变更实际执行 Red → Green → Refactor；已有行为抽取先跑 characterization，再迁移，不要求为了凑数量新增测试文件。

### 1. 先建立单一 HTTP + OAuth + 已有账号的最短闭环

改动范围：`go.mod`/`go.sum`、`internal/cli/root.go`、`internal/cli/commands/pixiv/mcp/`、`internal/cli/commands/fanbox/mcp/`、`internal/mcpserver/stdio.go`、两个产品的 registry/runtime，以及新增的窄 HTTP/auth owner。

1. 仅升级 MCP SDK 至 v1.8.0 及其必须的传递依赖；不执行全仓 `go get -u`，不添加外部 OAuth/IdP/前端依赖。同步已有第三方许可记录中实际发生的变化。
2. 先写失败测试：一个授权后的 `/mcp` 能同时发现 Pixiv/FANBOX；旧名与 download tools 不存在；未授权调用是 HTTP 401；stateless SDK 新/旧 revision 协商成功。
3. 将两套 registry 改为给同一个 `mcp.Server` 注册 tools，保留独立 runtime/SDK ports。tool 名在所属 package 正式改名，不在 handler 外套前缀兼容 adapter。删除旧 stdio wiring 和 fanbox mcp 命令，不删除 CLI 下载/编码器。
4. `internal/mcpserver/auth/` 负责具体 metadata/DCR/authorize/token handler 和一个具体 JSON state 实现；`internal/mcpserver` 负责 mux/bearer/生命周期；root 仅组装。测试 state 从真实临时目录重开，不用只会成功的内存假存储证明持久化。
5. Red 覆盖：错误 resource/redirect/PKCE、code 重用、refresh 重用、并发消费、写盘失败、reset 与 refresh 竞争、重启仍可 refresh、DCR client_id 不变。Green 后用标准客户端从 401 discovery 一直跑到真实 `tools/call`，检查不会把 401 转为成功-shaped MCP error。
6. 先部署这一闭环到单个既有 HTTPS 入口，在 ChatGPT 与 Gemini 的目标产品中添加同一个 URL，完成 DCR → owner 页 → host callback → token → 真实只读工具；再重启 server 验证原连接/refresh。通过后再展开自动新增账号和媒体 UI。未通过时只定位这一边界，不重构无关业务或引入 host 特判。

### 2. 接上共享账号与现有自动登录 relay

改动范围：`internal/mcpserver/pixiv/tools/account_list/`、`account_status/`、`account_use/`、`account_login_start/`；已有 Pixiv runtime、`internal/services/pixiv/account/`；上述 `loginrelay` 抽取；`internal/cli/commands/pixiv/auth/login_pair_relay.go` 及受影响的 helper/page 测试。

先固定四个工具的 input/output/schema；status 的 login_id 与 start 的 restart 按本文实现。新增 selection/current-account 的失败测试：不污染 CLI default；多账号无 default 要求选择；已选账号失效不 fallback；两 host 共享选择；在途操作不被换号；账号已入库但 selection 失败不能显示完全成功。

把现有 17 个聚焦登录/relay 测试作为抽取前后证据。为挂在主 mux 的 relay 增加集成测试：完整 session/start/callback/result、同一个 LoginSession、重复 start/restart、错误 proof/state、callback 并发、浏览器断开但服务器仍完成持久化、server shutdown 清理。不得增加 provider callback 接收 MCP tool，也不得增加固定登录时长。

真实验证分开执行：先用授权次要账号做已有 credential refresh/read；再在有 helper 的真实桌面完成一次新授权，不传 token、不粘贴 callback，并确认 account_status、账号库与后续 SDK read 一致。没有桌面授权条件时把第二项列为阻塞，不能拿第一项代替。

### 3. 先证明媒体真实交付，再完成 Gallery

改动范围：`internal/mcpserver/pixiv/tools/artwork_media/`、`internal/mcpserver/fanbox/tools/openresource/`、必要的窄 content 转换 helper、现有 SDK Resource 与 Ugoira pipeline 接口、一个内嵌 Gallery resource。

先用最小静态图片、两页作品、GIF/APNG、非图片附件验证完整字节/MIME与页码映射，接着在目标 host 验证实际展示/取得。媒体是否展示、动画是否播放、文件是否可下载分别记录；仅有 metadata、URL 或首帧不通过完整媒体交付。

Red 覆盖 regular/thumbnail/original 资源选择、不读取未选择页面、缺页/上游失败的 partial、账号资源隔离、真实 blob 与 MIME、调用取消后的资源清理。FANBOX 保持其认证 SDK，不通过 Pixiv runtime 或匿名 HTTP 获取付费资源；真实 FANBOX 凭证与测试附件需要独立明确授权。

基础媒体通过后加入 Gallery：标准 resource metadata/bridge、缩略图按需读取、分页、多页切换、收藏状态及错误状态；没有 UI extension 的客户端用原工具/媒体仍可完成同一任务。执行实际浏览器和 ChatGPT Gallery 验证，不凭 HTML 截图或本地 DOM 检查代替 host bridge 交互。

### 4. 反搜输入与合同收尾

改动范围：`internal/mcpserver/pixiv/tools/reverse_search/`、已有 facade/snapshot 对应测试、受影响的 registry contract，以及公开文档。

Red 先验证 openai/fileParams 精确 schema；source/image 的互斥；本地 regular file、file URI、HTTP URL仍可用；临时文件失败/取消清理；源 URL/路径/file_id 不泄露。只扩输入 normalization，不改 provider 网络策略，不引入新上传页面、代理服务或供应商 SDK。

真实上传到反搜 provider 需另外选择并授权一张图片；“允许使用 Pixiv 凭证”不是任意文件上传授权。当前凭证只用于授权的 refresh/read，不触碰收藏、关注或 FANBOX session。

同步 `docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md`、对应配置/开发文档和 `skills/pixiv-cli/` 中实际受影响的说明；README 保持简短。`AGENTS.md`、相关 maintainer skills 中的 stdio ownership 改成实际 HTTP ownership，其语言保持英文。删除过时安装示例/命令入口，不为旧名字保留 alias，也不为 docs-only 改动新增文档测试框架。

### 5. 验证命令和合入门禁

使用 `go.mod` 指定 Go；命令从仓库根目录执行。构建/E2E 显式提供可用 PATH、`CGO_ENABLED=1` 和现有目标 C 编译器，复用仓库已提交 Rust staticlib，不把 source build 误判为需要安装新的 Rust runtime。

```sh
# 抽取前后的最小登录回归；重命名/迁移后同步 package 路径，不丢失测试输入。
go test ./sdk/pixiv ./internal/cli/commands/pixiv/auth \
  ./internal/cli/commands/pixiv/auth/loginhelper \
  -run 'Test(LoginSession|OfficialOAuth|RemoteLogin|HandoffRelay|ConfiguredRelay|ValidateAuthorizationURL|ClearRemoteLogin|ForwardRemoteLogin)' \
  -count=1 -v

# 新增 HTTP/auth/媒体行为先执行实际新增的目标测试，再扩到受影响模块。
CGO_ENABLED=1 go test ./internal/mcpserver/... ./internal/services/pixiv/account/... -count=1

# 真实调用只在用户授权环境执行；账号来自已有私有存储，参数不放 token。
CGO_ENABLED=1 PIXIV_SDK_E2E=1 PIXIV_E2E_READ_USER_ID='<authorized-secondary-uid>' \
  go test ./e2e -run '^TestRealPixivSDKRead$' -count=1 -v

# 本次实现触及核心 runtime、共享账号和公开 MCP 合同，最终候选跑现有全量门禁。
CGO_ENABLED=1 go test ./... -count=1
CGO_ENABLED=1 go test -race ./... -count=1
CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 sh scripts/build.sh
git diff --check
```

先独立执行相关阶段收集阻塞，再运行预期可通过的总门禁；平台不支持的检查按仓库既有规则标明，不作通过。遵循现有 native/platform CI，Linux 通过不代表 Windows/macOS handler 或链接已验证。本轮只有文档修改，不要求为了“完成计划”把这些未来功能测试伪造出来。

## 2026-09-28 实际验证记录

本轮在上述基线的隔离 worktree 上审查现有实现，没有实现 remote OAuth/HTTP/媒体新功能，也没有修改依赖清单或数据库 schema。

| 实际执行 | 结果 | 证据范围 |
| --- | --- | --- |
| 服务器已安装 pixiv v1.1.0：次要账号 `auth check ... --json` | PASS | 凭证真实交换成功，credential revision 增加，CLI default 未变 |
| 上述三个 package 的登录/relay 过滤测试 | PASS，17 个顶层测试 | 当前 SDK callback 与自动 handoff 的 fixture 合同；含正确的无 state Pixiv 回调与错误 proof/跨 origin 路径 |
| 基线源码 `TestRealPixivSDKRead` | PASS，非 skip | refresh/持久化、CurrentUser 身份、SearchArtworks、Artwork、OpenResource HEAD；不代表图片 body/GIF 已交付 |
| ChatGPT/Gemini 对新服务的完整 OAuth、Gallery、上传反搜 | 未执行 | 新 HTTP/AS/媒体实现尚不存在，本会话也没有这两个目标产品的已授权浏览器会话 |
| 新账号真实 Pixiv 网页 → 桌面 helper → server 自动 callback | 未执行 | 已运行的是 relay fixture 与已有账号 refresh，不能把它们合称新账号登录成功 |

SDK E2E 初次编译因工具环境 CGO=0，随后因子进程缺失 PATH 而失败；补齐当前命令的 CGO/PATH 后，复用已有 gcc 与 committed staticlib，最终通过。未安装 Rust/浏览器/新 Go 依赖包；只下载 go.mod 已锁定的缺失依赖。没有输出 token、切换 CLI default、修改收藏/关注或向反搜 provider 上传图片。

当前证据支持复用 SDK、账号服务与既有 relay，而不是从零开发登录系统；**尚不支持宣称三家 host 的全自动授权已验收**。上述 live 阻塞必须由实现者真实完成后再勾选。

## 规范依据

- [Go MCP SDK releases](https://github.com/modelcontextprotocol/go-sdk/releases)：v1.8.0；2026-07-28 revision 的 stateless 要求。
- [MCP authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)：metadata discovery、resource、PKCE、DCR/CIMD 边界。
- [OpenAI authentication](https://developers.openai.com/plugins/build/auth)：ChatGPT 的 DCR、client_id 生命周期与回调/refresh。
- [OpenAI reference](https://developers.openai.com/plugins/reference)：openai/fileParams 的顶层 file object schema 与 UI metadata。
- [MCP Apps](https://py.sdk.modelcontextprotocol.io/advanced/apps/)：标准 UI resource/bridge，而非假设每个 host 展示能力相同。
- [RFC 6749 §4.1.2](https://datatracker.ietf.org/doc/html/rfc6749#section-4.1.2)、[RFC 6750 §5.3](https://datatracker.ietf.org/doc/html/rfc6750#section-5.3)、[RFC 9700 §4.14](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.14)：短期 code/access token、refresh rotation/reuse detection 的依据；不据此给业务请求加超时。

## 成功标准

本设计完成后：

- 用户只配置一个 remote MCP URL。
- ChatGPT、Gemini、Claude 看到同一套 Pixiv + FANBOX tools。
- 所有 Pixiv tools 命名明确属于 `pixiv_*`。
- MCP transport 不再存在 stdio 或旧版独立 SSE transport；保留官方 Streamable HTTP 自身所需的标准响应编码。
- 三家 connector 授权共享一个单 owner，但 Pixiv current account 可持久切换。
- 用户可以在聊天中真正看到/取得 Pixiv 单图、多图与 Ugoira，而不是只收到 server-local path 或 URL。
- 支持 MCP Apps 的 host 获得 Gallery；不支持的 host 忽略 UI 能力并仍有完整媒体能力，不需要用户配置开关。
- ChatGPT 可直接把用户上传图片用于反向搜图；其他 host 使用 HTTP(S) URL，本地 Agent 还可继续使用 local path / file://。
- CLI 的下载/归档能力继续存在，但 MCP 不再承担 server filesystem download contract。