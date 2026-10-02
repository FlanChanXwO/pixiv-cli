# MCP 工具

[English](../en/mcp-tools.md) | 简体中文 | [文档索引](../index-zh-CN.md)

通过 `pixiv mcp` 在 canonical `/mcp` endpoint 启动单一、经认证的 Pixiv/FANBOX Streamable HTTP server。MCP 使用自身 runtime 的凭据选择，不接受 CLI 数据命令的账号覆盖。JSON-RPC 经 HTTP 传输，不使用 stdin/stdout。详见 [HTTP 启动与代理配置](cli-reference.md#mcp-http-server)。`pixiv fanbox mcp`、stdio transport 与旧版独立 SSE transport 已删除；Streamable HTTP 仍可使用 SSE 响应。

`pixiv_novel_content` 保留正文不可用的错误合同；其 App API 正文 endpoint 已不可用。
传入正数 `novel_id` 时返回 structured `content_unavailable`、`isError=true` 和空
正文 block 列表；不会请求 `/v1/novel/content`，也不会 fallback 到 WebView。小说
metadata 请使用 `pixiv_novel_detail`。

独立本地命令 `pixiv mcp auth init [--reset]` 准备 owner state 并只输出一次 secret；它不是 tool，也不启动此 server。见 [owner 初始化](cli-reference.md#mcp-owner-初始化)。

## HTTP 授权

内置单 owner OAuth server 发布 protected-resource 与 authorization-server metadata，接受 DCR public client（`token_endpoint_auth_method=none`），使用 Authorization Code + PKCE S256，scope 为 `mcp`。owner 只在实例同意页输入本地初始化 secret，不放入聊天或 tool 参数；每次授权都须明确同意。云端部署使用 HTTPS 反向代理，本地支持 HTTP。该协议合同不代表 ChatGPT 或 Gemini 的真实连接验收已通过。

每次 `/mcp` 请求都须携带有效 bearer token。缺失、过期或撤销返回 HTTP 401 和 canonical `WWW-Authenticate` metadata，不伪装为 tool result；私有鉴权状态不可读时失败关闭。MCP 响应（含 SSE）使用 `Cache-Control: no-store`。access token 有效一小时；refresh 旋转 token pair，旧 refresh token 重放会撤销对应 grant。reset 撤销 grant 并使 owner session、未兑换 code 失效，但保留已注册 client；重启只丢弃内存授权 session/code，不丢失持久 client/grant。过期/reset 影响新请求，不中断已授权工作。服务取消时停止接收请求，取消并等待在途 handler；新版 revision 请求取消经官方 SDK 传播。

## Pixiv 账号选择

MCP owner 的 Pixiv 选择持久化保存，所有 connector 共享。首次使用优先采用 CLI 显式 default；没有显式 default 时，仅在本地恰有一个账号时自动采用。多个账号且无 default 返回 `selection_required`，没有本地账号返回 `no_local_account`。已有选择不会被隐式替换：账号已删除返回 `account_not_found`，缺少本地凭据返回 `credentials_missing`。本地凭据存在不代表上游实时有效。

一次 tool 调用在首次 SDK 访问时固定账号快照，后续读取和写入继续使用它。显式选择覆盖账号池调度，不修改 CLI default。通过 `pixiv_account_list`、`pixiv_account_status`、`pixiv_account_use` 查看或切换这个共享选择。

### 账号工具

| Tool | 输入与语义 |
| --- | --- |
| `pixiv_account_list` | 空 object；返回全部本地账号摘要及共享选择。 |
| `pixiv_account_status` | 空 object；查看本地选择与凭据是否存在，不联网 refresh。 |
| `pixiv_account_use` | 必填正整数 `user_id`；先检查本地存在，再持久化供所有 connector 共享。 |

输入均为封闭 object。三个工具均返回 `selected_user_id`、`selection_state`、`credential_state`、`accounts`（每项含 `user_id`、`username`、`has_credentials`）。状态查询可能持久化首次 default/唯一账号选择，因此三者均标注 `readOnlyHint=false`、`destructiveHint=false`、`idempotentHint=true`、`openWorldHint=false`。查询成功可以报告账号不可用，不代表上游认证成功。

失败保留 structured envelope、`error` 类别及 `isError=true`。类别为 `invalid_request`（schema）、`invalid_user_id`、`account_not_found`、`owner_not_initialized`、`canceled`、`deadline_exceeded` 或 `local_state_error`。状态不可用时 selection/credential state 为 `unknown`，不伪装为成功的空账号。错误结果不回显原始参数、凭据值、文件路径或内部存储原因。登录启动及 `login_id` 状态查询尚未开放。

## 错误、分页与输出

不符合 schema 的输入会在打开 SDK operation 前拒绝：MCP SDK 返回
`isError=true` 和文本诊断，不含 handler 的 structured output；这不是 JSON-RPC 协议错误。账号工具与反搜会为 schema 失败返回脱敏后的 structured 错误 envelope。
handler 执行后的失败会保留该 tool 的 structured result 并设置
`isError=true`：实体读取返回空 `records`。正常空页是成功，
不会被转换为错误。

列表 tool 接收 `page` 和 `limit`：

- 省略 `limit` 时读取一个上游批次。
- 正数 `limit` 会跨上游批次填充请求的逻辑页。
- `limit: 0` 会沿当前上游 cursor 读取至结束。
- `page` 从 1 开始，必须配合正数 `limit`。
- 实体 filter 在逻辑分页前执行，并按稳定实体身份去重。

`pixiv_illust_comments` 与 `pixiv_novel_comments` 发布封闭的 `{id, page, limit}` 输入 object。
`id` 必须为正数；输出 envelope 为 `{comments, pagination}`，并在上游明确提供时
附带 `total` 与 `access_control`，否则省略这些字段。作品评论与小说评论分别使用
当前的 comments operation。当当前 App API 提供 opaque numeric
`comment_access_control` wire 字段时，将其保留为
`access_control.comment_access_control`；SDK 与 server 不从该数值推断
`can_comment` 或 `is_locked`。两个 read tool 都不接受只用于 mutation 的
`stamp_id`，legacy MCP 注册表也不新增独立的 `stamps` tool。

SDK opaque cursor 不离开 server。列表结果提供 `pagination.page`、`limit`、
`returned`、`has_more`，适用时提供 `next_page`。`pixiv_recommended(kind="all")`
对 artwork、manga、novel、user 分别提供独立的分页对象。

Record 保留公开实体字段以及必要的 opaque resource reference，但不会输出已解析/签名资源 URL、
请求头、Cookie、过期 metadata、access token 或其他资源传输凭据。可用的小说正文 block、评论和
profile image 引用同样遵循此规则。
Structured result 使用显式 DTO 与 typed envelope，不直接编码 runtime SDK model。同一 server 的
FANBOX tools 也遵循相同资源形状：第一方 resource 只包含 opaque `ref` 与可选的
`requires_credentials`，不会包含 `url`、`request_headers` 或 `expires_at`。

## 反向搜图

`pixiv_reverse_search` 是 Pixiv MCP tool，输入是封闭 object：

```json
{"source":"/private/path/image.png","provider":"ascii2d-color"}
```

必须恰好提供 `source` 或 `image`；两者都不提供或同时提供均为错误。
`source` 可以是 server 本机常规文件路径、本机 `file://` URI 或 HTTP(S) URL。
为支持 host 附件，tool 发布 `_meta["openai/fileParams"] = ["image"]`：

```json
{"image":{"download_url":"https://host.example/image?signature=temporary","file_id":"host-file-id","mime_type":"image/png","file_name":"image.png"}}
```

`image` 是封闭 object，精确声明四个 string 字段：必填 `download_url`、`file_id`，可选
`mime_type`、`file_name`。下载 URL 只接受有 host、无 userinfo 的 HTTP(S)，不接受本地路径或 file URI。
`file_id` 不能为空，只是描述信息，不能当成下载地址。只有下载 URL 进入现有 snapshot loader；
文件 ID、MIME type 和文件名不会转发给 provider。host URL 过期或无法读取时，错误会提示用户重新附加图片；
server 不猜测其他 URL，也不重试。

本机 file URI 必须使用绝对路径，authority 只能为空或 `localhost`；其他 authority、userinfo、query、
fragment、NUL 和 UNC 路径会被拒绝，不能静默丢弃或重新解释。路径中的空格、`#`、`%` 等字面字符需要
percent-encode。这些 URI 规则仅适用于该 MCP 输入，不改变 CLI 的关键词/图片模式选择。

`provider` 可选。provider enum 为 `saucenao`、`ascii2d-color`、`ascii2d-bovw` 或 `all`；省略时
使用 MCP 进程启动时的配置，默认是 `saucenao`。`reverse_search_pixiv_only` 也在启动时固定，并控制
`results` 是否保留非 Pixiv 命中。单次 tool call 不能改变代理、API key 或其他传输配置。

启动时的 transport 有三个不同网络面：standard source/SauceNAO client、专用 ascii2d browser client，以及
FlareSolverr JSON control client。存在 `[reverse_search.network].proxy_url` 时（显式空值表示 direct access），
它只选择 ascii2d proxy；standard client 继续使用全局 proxy route。`[reverse_search.network].user_agent` 只作用于
ascii2d。Chromium User-Agent 会得到匹配的 `Sec-CH-UA`、`Sec-CH-UA-Mobile` 和 `Sec-CH-UA-Platform` hint，非
Chromium User-Agent 则省略这些 Chromium hint。`[reverse_search.flaresolverr].proxy_url` 只作为
`sessions.create` 中发送的 browser upstream proxy；solver control traffic 不继承任一 native route。

source 可以是 MCP server 上任意可读常规文件（路径或本机 file URI），或通过 server 网络抓取的 HTTP(S) URL，而非 connector
所在设备的文件或网络。允许私网、loopback、link-local URL 目标，server 也可以读取私有文件。单 owner OAuth
grant 允许 connector 请求这些 server 侧资源，因此只应授权可信 connector。server 只抓取或打开一次私有
快照，再上传给所选第三方 provider；不会在结果或日志中回显原始 source、附件下载 URL、文件 ID、文件名、临时路径、请求头、cookie、API key、CSRF 值、
redirect `Location` 或上游 response body。SauceNAO/ascii2d 的处理与保存遵循各自政策，URL 查询也可能被缓存；
ascii2d 接受 JPEG、PNG、WEBP，并执行 provider 自身的 10 MB 限制。

structured output 始终是封闭 envelope：`{input, providers, results, records, provider_errors, partial}`。
`input` 只包含 `kind` 和 `sha256`；`providers` 是固定 provider 状态列表；`results` 保留 provider evidence
和可选 canonical Pixiv identity；`provider_errors` 只包含稳定的 `provider`、`code`、`message`；`records` 包含
canonical `artwork` 或 `user` record。因为 provider 无法确定 Pixiv 作品 subtype，作品 record 刻意使用通用
类型，tool 不会调用作品详情来猜测。纯外部结果不会进入 `records`。

至少一个 provider 成功且另一个失败时，`partial=true`，tool 成功（`isError=false`）。单 provider 失败或全部
provider 失败时保留 envelope 并设置 `isError=true`；schema 错误在 provider 执行前返回安全的 `invalid_request` envelope，不回显无效值。快照在完成、失败或取消后删除。取消仍是完整请求取消，
不会伪装为 partial 成功。

图片由 native ascii2d `/search/file` multipart endpoint 上传；FlareSolverr 只接收 JSON challenge-recovery request，
绝不会收到图片上传。solver state 只属于进程/client，不持久化到磁盘。ascii2d 的 provider 自身 10 MB 限制不是
反向搜图全局 1 MiB 压缩上传规则；`gzip, deflate, br` 是 response negotiation。

当前稳定的反向搜图 error-code vocabulary 为：`unknown`、`invalid_request`、`invalid_source`、
`source_not_regular_file`、`source_read_failed`、`source_http_status`、`snapshot_failed`、
`source_loader_not_configured`、`provider_not_configured`、`missing_credential`、`malformed_upstream_response`、
`upstream_http_status`、`provider_failed`、`all_providers_failed`、`challenge_required`、`solver_unavailable`、
`solver_failed` 和 `malformed_solver_response`。provider failure cause 会在进入 `provider_errors` 前脱敏；只发布
经过审查的稳定 code 与安全 message。

## 实体 filter 与收藏数搜索

输入只包含下列 typed filter。原先的顶层表达式 `filter` 没有接入 handler，
因此不再发布；调用方应使用对应实体 filter。

| Filter | 字段 |
| --- | --- |
| `illust_filter` | `id`（正数）、`type`（`illust`、`manga` 或 `ugoira`）、`tags`（全部精确匹配）、`min_views`（非负）、`min_pages`（非负） |
| `novel_filter` | `id`（正数）、`tags`（全部精确匹配）、`min_views`（非负） |
| `user_filter` | `id`（正数） |

Artwork search 另外接受 `bookmark_min`、`bookmark_max` 和
`bookmark_strategy`（`auto`、`local`、`best_effort`、`server`）。范围是非负闭区间。
application outcome 的 `filter` 会报告 `min`、`max`、`membership`、`strategy` 和
`completeness`：

- `auto` 当前解析为对已取得候选的 `TotalBookmarks` 本地筛选。
- `local` 执行同样的候选精确筛选，但不声称全局结果完备。
- `best_effort` 保留 App candidate bounds，并报告 partial completeness。
- `server` 在可靠服务端行为有证据前显式失败，不会静默切换其他策略。

未知 membership 不等于 non-Premium。Premium 不是本地硬门槛，收藏数也不得称为点赞数。

## Tool annotations 与下载

Pixiv tools 统一使用 `pixiv_` 前缀，FANBOX 使用 `fanbox_`；不注册旧 Pixiv 名称 alias。两产品注册到同一 server，SDK runtime 和凭据选择保持独立。

所有 tools 显式发布标准 annotations。账号工具的首次初始化副作用见上文；其他读取为只读、非破坏、幂等；新增收藏/关注为非破坏、幂等写入；取消收藏/关注及删除评论为破坏、幂等写入；创建/回复/盖章评论为非破坏、非幂等写入。这些 hints 供 host 审批使用，不是 server 强制鉴权。

外部操作的 `openWorldHint` 为 true，包括上传至第三方的反向搜图及可能触发认证的 Pixiv 读取。`pixiv_account_list`、`pixiv_account_status`、`pixiv_account_use` 是 closed-world 本地操作。`fanbox_resolve_url` 也为 closed-world：打开本地账号快照并解析 URL 不产生网络访问。

MCP 不再注册 `download`、`download_random_from_recommendation` 或任何改名后的下载 alias。需要写入本地文件系统时使用 CLI `pixiv download`。

## 读取工具

| Tool | 输入与语义 |
| --- | --- |
| `pixiv_search_illust` | 必填 `word`；可选 `search_target`、`sort`、`duration`、`start_date`、`end_date`、`content_type`、`ai_mode`、`aspect_ratio`、`resolution`、精确 `tool`、收藏范围/策略、`illust_filter`、`page`、`limit`。稳定 enum/date 会在打开 SDK 前校验。 |
| `pixiv_search_novel` | 必填 `word`；可选 `search_target`、`sort`、`duration`、`novel_filter`、`page`、`limit`。rating、正文长度和 original 字段明确不发布。 |
| `pixiv_reverse_search` | `source`（server 本机常规文件、本机 `file://` URI、HTTP(S) URL）与 host `image` object 恰好提供一个；可选 `provider` enum。使用启动时固定的代理/key/pixiv-only 配置，返回上文的反向搜图 envelope。 |
| `pixiv_illust_detail` | 正数 `illust_id` 与受支持作品 `url` 必须二选一；返回一条安全 record。 |
| `pixiv_novel_detail` / `pixiv_novel_content` | 正数 `novel_id`；前者返回 metadata，后者是保留的兼容 tool，返回 `content_unavailable` 与空 block，不请求已 rejected 的正文 endpoint。 |
| `pixiv_illust_related` | 正数 `illust_id`，可选 `illust_filter`、`page`、`limit`。 |
| `pixiv_illust_series` / `pixiv_novel_series` | 正数 `series_id`、`page`、`limit`；小说系列额外返回安全 series metadata。 |
| `pixiv_illust_comments` / `pixiv_novel_comments` | 封闭输入 `{id, page, limit}`，其中 `id` 为正数；输出 `{comments, pagination}`，并可选返回 `total`/`access_control` metadata。opaque numeric `comment_access_control` 会保留在 `access_control` 内，不推断布尔权限。read tool 不接受只用于 mutation 的 `stamp_id`，legacy 注册表不暴露独立 `stamps` tool。 |
| `pixiv_illust_ranking` | 可选 `mode`、`date`、`illust_filter`、`page`、`limit`；`mode` 是封闭的 ranking enum，日期必须是有效 `YYYY-MM-DD`，省略 mode 为 `day`。 |
| `pixiv_search_user` | 必填非空白 `word`，可选 `user_filter`、`page`、`limit`；空白输入会在 SDK 执行前拒绝，合法输入调用 App user-search operation。 |
| `pixiv_illust_recommended` | 作品推荐，可选 `illust_filter`、`page`、`limit`。 |
| `pixiv_recommended` | 必填 `kind`：`all`、`illust`、`manga`、`novel` 或 `user`；可选匹配的 typed filter、`page`、`limit`。`illust`/`manga` 选择对应 artwork subtype，冲突 filter 会在 SDK 执行前拒绝；`all` 保持四路独立流，并采用原子失败语义。 |
| `pixiv_trending_tags_illust` | 无输入；返回完整当前作品趋势标签列表。上游返回空列表时仍是成功的空结果。 |
| `pixiv_timeline_illust_following` / `pixiv_timeline_novel_following` | `restrict`（`public`/`private`）、匹配实体 filter、`page`、`limit`。 |
| `pixiv_timeline_illust_latest` | 必填 `content_type`（`illust` 或 `manga`），可选 `illust_filter`、`page`、`limit`。 |
| `pixiv_timeline_novel_latest` | 可选 `novel_filter`、`page`、`limit`。 |
| `pixiv_mypixiv_users` | 可选 `user_filter`、`page`、`limit`。 |
| `pixiv_mypixiv_illusts` / `pixiv_mypixiv_novels` | 对应 typed filter、`page`、`limit`。 |
| `pixiv_user_detail` | 必填正数 `user_id`；返回一条安全公开 profile record。 |
| `pixiv_user_artworks` | 可选 `user_id`、`type`（`illust`、`manga`、`ugoira`）、`illust_filter`、`page`、`limit`；省略 ID 使用认证账号。 |
| `pixiv_user_novels` | 可选 `user_id`、`novel_filter`、`page`、`limit`；省略 ID 使用认证账号。 |
| `pixiv_user_bookmarks` | 可选 `user_id`、`restrict`、`tag`、`illust_filter`、`page`、`limit`；读取作品收藏。 |
| `pixiv_user_novel_bookmarks` | 可选 `user_id`、`restrict`、`tag`、`page`、`limit`；读取小说收藏。 |
| `pixiv_bookmark_list_all` | 可选 `user_id`、`restrict`、`tag`、`page`、`limit`；新增的聚合 tool，先读取作品收藏再读取小说收藏。`page`/`limit` 作用于拼接后的统一逻辑流；任一 required 流失败时返回错误，不返回部分 records。 |
| `pixiv_bookmark_tags_all` | 可选 `user_id`、`restrict`、`page`、`limit`；新增的聚合 tool，先读取作品标签再读取小说标签。每个标签保留 `content_type` 与原始 `count`；同名标签不合并，任一 required 流失败时不返回部分标签。 |
| `pixiv_novel_bookmark_tags` | 可选 `user_id`、`restrict`、`page`、`limit`；返回小说收藏的 `{bookmark_tags, pagination}`。当前 candidate App API 没有续页 contract；超出该 contract 的 continuation 会返回 typed error。 |
| `pixiv_novel_bookmark_detail` | 必填正数 `novel_id`；返回单篇小说的 `{bookmarked, restrict, tags}`，保留缺失/未收藏状态；使用 candidate novel-bookmark detail App API。 |
| `pixiv_user_following` | 可选 `user_id`、`restrict`、`user_filter`、`page`、`limit`；省略 ID 使用认证账号。 |
| `pixiv_user_followers` | 可选 `user_id`、`restrict`、`page`、`limit`；省略 ID 使用认证账号。 |
| `pixiv_related_users` | 可选正数 `user_id`（省略时使用认证账号），兼容字段 `restrict`，可选 `user_filter`、`page`、`limit`。 |
| `pixiv_blocked_users` | 可选 `user_id`、兼容字段 `restrict`、`page`、`limit`；省略 ID 使用认证账号。App API 失败会显露，不切换 Web fallback。 |
| `pixiv_bookmark_tags` | 可选 `user_id`、`restrict`、`page`、`limit`；返回 `{bookmark_tags, pagination}`。 |
| `pixiv_bookmark_detail` | 必填正数 `illust_id`；返回 `{bookmarked, restrict, tags}`，保留未收藏状态。 |

所有读取 tool 共用 application/public SDK 路径，保留认证、授权、not found、上游、取消和 malformed response
等 typed error。可选 port 缺失或 App 请求失败不会伪造空成功结果。

## 写工具

| Tool | 输入 | Structured output |
| --- | --- | --- |
| `pixiv_add_bookmark` | `illust_id`，可选 `restrict`、可重复 `tags` | `{success, action, illust_id}` |
| `pixiv_add_novel_bookmark` | `novel_id`，可选 `restrict`、可重复 `tags` | `{success, action, novel_id}` |
| `pixiv_remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `pixiv_remove_novel_bookmark` | `novel_id` | `{success, action, novel_id}` |
| `pixiv_create_artwork_comment` | 正数 `illust_id`、非空 `comment` | `{success, action, illust_id, comment_id}` |
| `pixiv_reply_artwork_comment` | 正数 `illust_id`、非空 `comment`、正数 `parent_comment_id` | `{success, action, illust_id, comment_id}` |
| `pixiv_stamp_artwork_comment` | 正数 `illust_id`、可选 `comment`（sticker-only 时为空）、正数 `stamp_id` | `{success, action, illust_id, comment_id}` |
| `pixiv_delete_artwork_comment` | 正数 `comment_id` | `{success, action, comment_id}` |
| `pixiv_create_novel_comment` | 正数 `novel_id`、非空 `comment` | `{success, action, novel_id, comment_id}` |
| `pixiv_reply_novel_comment` | 正数 `novel_id`、非空 `comment`、正数 `parent_comment_id` | `{success, action, novel_id, comment_id}` |
| `pixiv_stamp_novel_comment` | 正数 `novel_id`、可选 `comment`（sticker-only 时为空）、正数 `stamp_id` | `{success, action, novel_id, comment_id}` |
| `pixiv_delete_novel_comment` | 正数 `comment_id` | `{success, action, comment_id}` |
| `pixiv_follow_user` | `user_id`，可选 `restrict` | `{success, action, user_id}` |
| `pixiv_unfollow_user` | `user_id` | `{success, action, user_id}` |

写操作包含作品/小说收藏、作品/小说评论/印章评论和用户关注 mutation。add 省略 `restrict` 时默认为
`public`，只接受 `public` 或 `private`。作品与小说评论 create/reply/stamp 会直接返回上游给出的
`comment_id`；delete 返回输入的评论 ID。server 不会读取最新评论来猜测 ID，不会回退到 candidate
v3 comments contract，提交后状态未知时不会换账号重放；失败写操作返回 `success=false`、
`isError=true` 和安全诊断。

## FANBOX tools

同一 server 注册以下名称不变的 FANBOX tools。完整输入 schema 由 `tools/list` 提供，凭据仍归 FANBOX 独立管理。

| Tools | 语义 |
| --- | --- |
| `fanbox_current_user` | 当前已认证 FANBOX 用户。 |
| `fanbox_creator`、`fanbox_creators` | 单个创作者 profile，或支持中/关注中的创作者。 |
| `fanbox_creator_tags` | 创作者使用的标签。 |
| `fanbox_creator_posts`、`fanbox_tagged_posts` | 创作者或某个创作者标签下的帖子。 |
| `fanbox_post` | 单个帖子及安全 resource refs。 |
| `fanbox_home`、`fanbox_supporting` | 已认证主页/支持中 feed。 |
| `fanbox_resolve_url` | 在本地将 URL 解析成 typed reference。 |
| `fanbox_open_resource` | 使用 `GET` 或 `HEAD` 打开 opaque `ref`；当前仅返回状态、Content-Type 和长度，不交付媒体 bytes。 |

## 认证与 fallback

Pixiv 读写要求配置好的 App API access path。不存在匿名或 Web fallback，App API 错误即为最终错误。若配置仍含已移除的
`web_fallback_enabled`，会返回 `removed_setting`。

FANBOX tools 共享协议 server，不共享 Pixiv 凭据或账号池。两产品保留各自 SDK 与 service 配置；命令级 proxy override 对两者的原生连接生效。
