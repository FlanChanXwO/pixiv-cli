# MCP 工具

[English](../en/mcp-tools.md) | 简体中文 | [文档索引](../index-zh-CN.md)

通过 `pixiv mcp` 启动 Pixiv stdio MCP server。MCP 使用自身 runtime 的凭据选择，
不接受 CLI 数据命令的账号覆盖；stdout 始终保留给 JSON-RPC。

## 错误、分页与输出

不符合 schema 的输入会在打开 SDK operation 前作为 JSON-RPC/tool input error
拒绝。handler 执行后的失败会保留该 tool 的 structured result 并设置
`isError=true`：实体读取返回空 `records`，下载保留下载报告形状。正常空页是成功，
不会被转换为错误。

列表 tool 接收 `page` 和 `limit`：

- 省略 `limit` 时读取一个上游批次。
- 正数 `limit` 会跨上游批次填充请求的逻辑页。
- `limit: 0` 会沿当前上游 cursor 读取至结束。
- `page` 从 1 开始，必须配合正数 `limit`。
- 实体 filter 在逻辑分页前执行，并按稳定实体身份去重。

SDK opaque cursor 不离开 server。列表结果提供 `pagination.page`、`limit`、
`returned`、`has_more`，适用时提供 `next_page`。`recommended(kind="all")`
对 artwork、manga、novel、user 分别提供独立的分页对象。

Record 保留公开实体字段以及必要的 opaque resource reference，但不会输出已解析/签名资源 URL、
请求头、Cookie、过期 metadata、access token 或其他资源传输凭据。小说正文 block、评论和 profile image
引用同样遵循此规则。
Structured result 使用显式 DTO 与 typed envelope，不直接编码 runtime SDK model。独立的
FANBOX MCP server 也遵循相同资源形状：第一方 resource 只包含 opaque `ref` 与可选的
`requires_credentials`，不会包含 `url`、`request_headers` 或 `expires_at`。

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

## 下载工具

| Tool | 输入 | Structured output |
| --- | --- | --- |
| `download` | `src` 与非空 `srcs` 必须二选一；每项是作品 PID、受支持的 Pixiv 作品/用户/公开收藏 URL 或允许的 CDN URL。可选 `pages` 使用 `1,3-5` 这类闭区间，`quality` 为 `original`、`regular`、`small`、`thumb`、`mini`，`ugoira_mode` 为 `gif` 或 `apng`，`delivery` 为 `local_path`。 | `{delivery, items, failures, files, text}`；每个 file 包含安全的本地 path/URI、MIME、大小和页码。任何失败都会保留 failure 并设置 `isError=true`。 |
| `download_random_from_recommendation` | 可选 `count`（默认 `5`，显式值 `1..20`）、`pages`、`quality`、`ugoira_mode` 和 `delivery: "local_path"`。 | 同样的本地文件报告结构。 |

当前 MCP handler 没有把并发、filter、archive、directory-template、metadata sidecar、重试次数或重试延迟
映射到 `DownloadRequest`，因此 download schema 不发布这些字段。下载使用已配置的 application 路径/模板，
选项错误会显露，不会静默忽略输入。

用户和公开收藏 URL 按来源顺序展开认证态视觉作品，不包含小说；插画系列 URL 不是下载来源。URL 在本地解析，不抓 HTML
或跟随重定向；CDN source 没有作品 metadata，不会被当作作品详情请求。

## 读取工具

| Tool | 输入与语义 |
| --- | --- |
| `search_illust` | 必填 `word`；可选 `search_target`、`sort`、`duration`、`start_date`、`end_date`、`content_type`、`ai_mode`、`aspect_ratio`、`resolution`、精确 `tool`、收藏范围/策略、`illust_filter`、`page`、`limit`。稳定 enum/date 会在打开 SDK 前校验。 |
| `search_novel` | 必填 `word`；可选 `search_target`、`sort`、`duration`、`novel_filter`、`page`、`limit`。rating、正文长度和 original 字段明确不发布。 |
| `illust_detail` | 正数 `illust_id` 与受支持作品 `url` 必须二选一；返回一条安全 record。 |
| `novel_detail` / `novel_content` | 正数 `novel_id`；前者返回 metadata，后者返回完整结构化正文 block。 |
| `illust_related` | 正数 `illust_id`，可选 `illust_filter`、`page`、`limit`。 |
| `illust_series` / `novel_series` | 正数 `series_id`、`page`、`limit`；小说系列额外返回安全 series metadata。 |
| `illust_comments` / `novel_comments` | 正数作品/小说 `id`、`page`、`limit`；输出安全 comments、pagination，以及可取得的 `total`/`access_control` metadata。 |
| `illust_ranking` | 可选 `mode`、`date`、`illust_filter`、`page`、`limit`；省略 mode 为 `day`。 |
| `search_user` | 必填 `word`，可选 `user_filter`、`page`、`limit`；调用 App user-search operation。 |
| `illust_recommended` | 作品推荐，可选 `illust_filter`、`page`、`limit`。 |
| `recommended` | 必填 `kind`：`all`、`illust`、`manga`、`novel` 或 `user`；可选匹配的 typed filter、`page`、`limit`。 |
| `trending_tags_illust` | 无输入；返回完整当前作品趋势标签列表。 |
| `timeline_illust_following` / `timeline_novel_following` | `restrict`（`public`/`private`）、匹配实体 filter、`page`、`limit`。 |
| `timeline_illust_latest` | 必填 `content_type`（`illust` 或 `manga`），可选 `illust_filter`、`page`、`limit`。 |
| `timeline_novel_latest` | 可选 `novel_filter`、`page`、`limit`。 |
| `mypixiv_users` | 可选 `user_filter`、`page`、`limit`。 |
| `mypixiv_illusts` / `mypixiv_novels` | 对应 typed filter、`page`、`limit`。 |
| `user_detail` | 必填正数 `user_id`；返回一条安全公开 profile record。 |
| `user_artworks` | 可选 `user_id`、`type`（`illust`、`manga`、`ugoira`）、`illust_filter`、`page`、`limit`；省略 ID 使用认证账号。 |
| `user_novels` | 可选 `user_id`、`novel_filter`、`page`、`limit`；省略 ID 使用认证账号。 |
| `user_bookmarks` | 可选 `user_id`、`restrict`、`tag`、`illust_filter`、`page`、`limit`；读取作品收藏。 |
| `user_novel_bookmarks` | 可选 `user_id`、`restrict`、`tag`、`page`、`limit`；读取小说收藏。 |
| `user_following` / `user_followers` | 可选 `user_id`、`restrict`、`user_filter`、`page`、`limit`；省略 ID 使用认证账号。 |
| `related_users` | 正数 `user_id`，可选 `user_filter`、`page`、`limit`。 |
| `blocked_users` | 可选 `user_id`、`page`、`limit`；省略 ID 使用认证账号。App API 失败会显露，不切换 Web fallback。 |
| `bookmark_tags` | 可选 `user_id`、`restrict`、`page`、`limit`；返回 `{bookmark_tags, pagination}`。 |
| `bookmark_detail` | 必填正数 `illust_id`；返回 `{bookmarked, restrict, tags}`，保留未收藏状态。 |

所有读取 tool 共用 application/public SDK 路径，保留认证、授权、not found、上游、取消和 malformed response
等 typed error。可选 port 缺失或 App 请求失败不会伪造空成功结果。

## 写工具

| Tool | 输入 | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`，可选 `restrict`、可重复 `tags` | `{success, action, illust_id}` |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `follow_user` | `user_id`，可选 `restrict` | `{success, action, user_id}` |
| `unfollow_user` | `user_id` | `{success, action, user_id}` |

写操作只包含作品收藏和用户关注 mutation。提交后状态未知时不会换账号重放；失败写操作返回
`success=false`、`isError=true` 和安全诊断。

## 认证与 fallback

Pixiv 读写要求配置好的 App API access path。不存在匿名或 Web fallback，App API 错误即为最终错误。若配置仍含已移除的
`web_fallback_enabled`，会返回 `removed_setting`。

FANBOX 使用独立 MCP server，不共享 Pixiv 凭据、代理设置、tool 或 route。
