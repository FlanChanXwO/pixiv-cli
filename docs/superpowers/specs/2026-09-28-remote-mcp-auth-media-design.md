# Remote MCP、鉴权与媒体交付设计

日期：2026-09-28

## 目标

将 pixiv-cli 的 MCP 从本地 stdio 模式重构为面向自托管远程连接器的单一 MCP 服务，使 ChatGPT、Gemini 与 Claude 能通过同一个公开 HTTPS 地址连接，并完整使用 Pixiv 与 FANBOX 能力。

本次是明确的 breaking change：

- 只保留 Streamable HTTP，不保留 stdio、SSE 或 transport 兼容层。
- Pixiv 与 FANBOX 合并到一个 MCP endpoint。
- Pixiv tools 全部增加 `pixiv_` 前缀；FANBOX tools 保持 `fanbox_` 前缀。
- MCP 不再提供“下载到 MCP server 本地磁盘”的 download tools；媒体通过 MCP 实际交付给 host。
- 内置单用户 OAuth 2.1 Authorization Server，通过 DCR + Authorization Code + PKCE 供 ChatGPT、Gemini、Claude 连接。

## 非目标

- 不做多用户、组织、角色或权限矩阵。
- 不做 read/write/admin OAuth scopes；tool 调用审批交给 host。
- 不为某一家 host 写 transport 或 OAuth 特判。
- 不保留旧 tool name alias。
- 不实现 SSE 或 stdio fallback。
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
- `reverse_search` 接受 server 本地文件路径或 HTTP(S) URL，符合 trusted local stdio 边界，但不适合 remote MCP。

Remote MCP 后，上述本地文件语义全部失效：ChatGPT/Gemini/Claude 无法访问 MCP server 的 `file://`，用户上传的本地图片也不等于 server 本地路径。

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
- OAuth issuer、metadata URL、redirect/resource URL 全部只从配置的 canonical base URL 构造，不信任请求的 `Host` / `X-Forwarded-*` 来决定 URL。
- 当服务需要给 ChatGPT/Gemini/Claude 这类云端 connector 使用时，canonical base URL 配置为公网 HTTPS 地址，并由 Caddy、nginx、Cloudflare 或其他现有反向代理负责 TLS。
- 三家云端 connector 使用同一个公开 `https://.../mcp`；本地 Agent 不需要为了使用同一 Streamable HTTP server 额外部署 HTTPS。

## MCP SDK 与协议

升级到当前稳定的 `github.com/modelcontextprotocol/go-sdk` v1 系列；设计验证基线为 2026-09-28 的 v1.8.0。

Transport 只有 Streamable HTTP。协议 revision 使用官方 SDK 的标准协商能力，不自行实现旧协议兼容层，也不因某个 host 维护独立 transport。

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

本地恢复命令：

```text
pixiv mcp auth reset
```

重新生成 owner secret，并使所有未完成 authorization code、现有 access token 与 refresh token 失效；DCR client registration 可保留，使 ChatGPT/Gemini/Claude 重新授权时不必强制重新注册 client。该命令只能在 MCP server 所在机器本地执行。

OAuth authorize 页面要求用户输入 owner secret。该 secret：

- 不进入 MCP tool arguments。
- 不提供给 LLM。
- 不作为 OAuth bearer token。
- 不等同于 Pixiv/FANBOX credential。

### OAuth endpoints

公共 HTTP surface：

```text
GET  /.well-known/oauth-protected-resource
GET  /.well-known/oauth-authorization-server
POST /oauth/register
GET  /oauth/authorize
POST /oauth/authorize
POST /oauth/token
POST /mcp
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

为三家共同兼容，第一版只暴露 DCR，不实现 CIMD；不写 `if ChatGPT` / `if Gemini` / `if Claude` 分支。

OAuth scope 第一版只有：

```text
mcp
```

不构建 `mcp:read` / `mcp:write` / `mcp:admin`。

## Tool 权限与 host 审批

权限等级由 ChatGPT/Gemini/Claude host 控制，不由 OAuth scope 重复实现。

每个 tool 必须准确设置标准 annotations，例如：

- 纯读取：`readOnlyHint=true`。
- 可逆写入（收藏、关注、账号切换）：`readOnlyHint=false`, `destructiveHint=false`。
- 删除/撤销类操作：`readOnlyHint=false`, `destructiveHint=true`。
- 重复执行安全的操作按真实语义设置 `idempotentHint=true`。
- `openWorldHint` 按 tool 是否访问外部网络真实标注。

这些 annotation 只向 host 描述行为，不替代 server 的认证和输入校验。

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
pixiv_account_login_complete
```

语义：

- 选择结果持久化在 SQLite 的 MCP owner state，不写入 CLI 的 `[pixiv.auth].default_user_id`。
- ChatGPT 调 `pixiv_account_use(B)` 后，Gemini 与 Claude 后续调用也使用 B。
- 显式 MCP current account 覆盖 read pool 自动轮换；MCP 的读取和写入都使用该账号，避免用户切换后仍被 pool 偷偷换号。
- 如果 selected account 被删除、过期或不可用，不静默切到其他账号；`pixiv_account_status` 返回明确状态。
- 成功登录新账号且当前没有可用 selected account 时，将新账号设为 MCP current account；已有可用 current account 时不隐式切换。

## Pixiv 登录

继续复用现有 public SDK `LoginSession` / `LoginService`：

- PKCE verifier、OAuth state、Pixiv access/refresh token 永远留在 pixiv-cli。
- MCP tool 不返回 Pixiv token。
- MCP tool 不接受 raw refresh token。

基本流程：

```text
pixiv_account_login_start
  → login_id
  → Pixiv authorization_url
  → user opens URL in current browser
  → Pixiv fixed callback page
  → callback URL returned to pixiv_account_login_complete
  → account persisted
```

Pixiv App OAuth 的 callback 不是 pixiv-cli 可自由设置的 HTTPS callback，因此核心协议不承诺浏览器自动把最终 callback 送回 server。

允许 host 自动化这一步，但核心 contract 不依赖它：

- 如果 host 能可靠获取最终 callback URL，可自动调用 `pixiv_account_login_complete`。
- 否则用户复制完整 callback URL 回到聊天，模型调用 `pixiv_account_login_complete`。

不要求桌面机器安装 pixiv-cli deep-link handler 才能完成 remote MCP 登录。

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

返回同时包含：

1. structured metadata：illust ID、title、page、MIME、size 等。
2. MCP `ImageContent`：真实图片 bytes，使 ChatGPT/Gemini/Claude 可以实际展示或交给本地 agent 保存。

多图作品必须可以取得全部图片。不能静默只返回第一页；若 host/transport 的真实限制阻止一次完整传输，应返回明确失败/未完成状态，并允许按页再次读取，不做无提示截断。

### Ugoira

复用现有 Ugoira pipeline 与 Rust encoder：

- 默认 GIF。
- APNG 可选。
- 不新增 ffmpeg runtime dependency。
- 返回实际动画媒体内容，而不是只返回 Pixiv URL 或 server local path。

Host 是否内联播放动画属于 host 展示能力；MCP 必须至少实际交付动画文件/内容，而不是只报告“已找到”。

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

`image` 参数接收 host 提供的临时 file descriptor（例如 download URL、file ID、MIME、filename）。pixiv-cli 下载临时文件内容后交给现有 reverse-search facade。

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

### 4. Browser upload fallback

对于不能把用户上传文件传给 custom MCP 的 host，同一个 `pixiv_reverse_search` 提供一次性 browser upload fallback：

- 调用时没有 `image` 或 `url`，server 创建 one-time upload session。
- 返回 `upload_id` 与 HTTPS `upload_url`。
- 用户在任意浏览器打开 URL 并选择图片。
- 上传完成后，再次调用同一个 `pixiv_reverse_search` 并提供 `upload_id`。
- session 只能消费一次，不能成为通用文件托管。
- session 带 `created_at` / `expires_at` 并自动清理；生命周期只覆盖一次正常浏览器上传，不暴露为用户可调的通用 session 配置。
- 上传内容只用于该次 reverse search，不进入持久作品归档。

若 negotiated MCP/host 支持 URL elicitation，可用它改善打开 URL 的体验；核心 fallback 不依赖 elicitation 或 MCP Apps。

Gemini 的核心验收使用 URL / browser-upload fallback，不假定 Gemini 会把 Spark task 上传图片直接转发给第三方 MCP。

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
- reverse-search upload URL 是 one-time capability，不提供目录浏览或任意文件读取。
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

继续使用现有 SQLite + config 分工：

SQLite 持久化：

- owner secret verifier。
- DCR client metadata。
- one-time authorization code state。
- opaque access/refresh token verifier 与 grant metadata。
- singleton MCP owner state（至少包含 selected Pixiv account）。
- short-lived reverse-search upload session state。

config 保存非 secret 服务配置：

- MCP listen address。
- public HTTPS base URL。
- 现有 proxy / reverse-search provider 配置继续沿用既有 owner。

不要引入泛化 IAM、通用 session framework 或额外数据库。

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
- reverse-search HTTP URL 与 one-time browser upload flow。
- Gallery tool/resource contract 在无 UI host 时仍保持完整 structured/media fallback。

### ChatGPT live E2E — blocking

必须实测：

1. 通过 custom third-party remote MCP 添加 `/mcp`。
2. DCR + OAuth owner authorization 完成。
3. tool discovery 正常，旧名不存在。
4. Pixiv account list/use/status 可用且重连后选择保持。
5. Pixiv 登录可以 start，并通过自动 callback（若 host 能做到）或手工粘贴 callback 完成。
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
6. reverse search 可通过 HTTP URL 或 browser-upload fallback 完成。

不以 Gemini MCP Apps Gallery 作为验收项。

### Claude

按同一 Streamable HTTP、DCR/OAuth、tool/media、MCP Apps 标准实现；不要求本次 release 前进行 live E2E，不作为 merge/release blocker。

## 成功标准

本设计完成后：

- 用户只配置一个 remote MCP URL。
- ChatGPT、Gemini、Claude 看到同一套 Pixiv + FANBOX tools。
- 所有 Pixiv tools 命名明确属于 `pixiv_*`。
- MCP transport 不再存在 stdio/SSE/旧 transport。
- 三家 connector 授权共享一个单 owner，但 Pixiv current account 可持久切换。
- 用户可以在聊天中真正看到/取得 Pixiv 单图、多图与 Ugoira，而不是只收到 server-local path 或 URL。
- 支持 MCP Apps 的 host 获得 Gallery；不支持的 host 忽略 UI 能力并仍有完整媒体能力，不需要用户配置开关。
- ChatGPT 可直接把用户上传图片用于反向搜图；其他 host 至少有标准 URL / browser upload fallback。
- CLI 的下载/归档能力继续存在，但 MCP 不再承担 server filesystem download contract。