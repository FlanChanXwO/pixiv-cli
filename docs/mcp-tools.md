# MCP 工具

以 `pixiv mcp` 启动 stdio server。stdout 仅用于 JSON-RPC，日志写 stderr。MCP 不提供 HTTP endpoint。

有 refresh token 时 App API 为主路径，失败不自动回落 Web；无 refresh token 且 `web_fallback_enabled=true` 时，仅匿名白名单读 tool 可用 Web API。SDK 路径的用户详情、用户列表和收藏/关注写操作同时返回文本内容与 structured output；其可分类失败会令 result `isError=true`，保留安全错误文本和对应 structured output。遗留 MCP tool 保持既有文本结果兼容，不承诺统一 `isError` 语义。

## 分页

新 SDK 列表 tool 均使用：

- `limit`：最大项目数；`0` 表示跟随上游直到没有下一批；不传时兼容为一个上游批次。
- `page`：从 1 开始的逻辑页，必须配正数 `limit`。
- 输出 `pagination.page`、`limit`、`returned`、`has_more`、可选 `next_page`。

SDK cursor 不出现在 MCP 参数或输出。`user_bookmarks.max_bookmark_id` 是旧 continuation，已废弃，不能与 `page` 或 `limit` 同用。`user_following.offset` 是旧逻辑 offset，已废弃，仅与 `page` 互斥，允许和 `limit` 同用。

## 配置、认证与下载

| tool | 参数 | structured output |
| --- | --- | --- |
| `set_download_path` | `path` | 文本状态。 |
| `refresh_token` | 无 | 当前认证账号摘要。 |
| `set_refresh_token` | 原始 App API `refresh_token` | 当前会话认证结果；不写 `auth.json`；Cookie 输入会被拒绝。 |
| `download` | `illust_id` 或 `illust_ids`，可选 `delivery` | 下载文件、URI、MIME、大小；`image_content` 另附 ImageContent。 |
| `download_random_from_recommendation` | 可选 `count`（省略或 `null` 时默认 5；显式值须为 1..20），可选 `delivery` | 下载结果文本与 structured 文件元数据；不附加 ImageContent。 |

`download_random_from_recommendation.count` 限制本次请求的作品数，不限制一个作品展开的文件数。显式传入 0、负数或大于 20 的值会返回参数错误，不会改写为默认值或边界值；推荐列表少于请求数时则下载列表中实际可用的作品。该 tool 当前返回下载结果文本与 structured 文件元数据，不会像 `download` 的 `delivery=image_content` 路径那样附加 ImageContent。

两个下载 tool 在参数校验、SDK、推荐获取、下载、结果整理或文件读取失败时，都会保留原有业务错误文本，并返回有效 structured output：`delivery` 保留已规范化的交付方式（无 ID 或非法 `delivery` 时为 `local_path`），`items` 与 `files` 是空数组而不是 `null`。这些遗留失败结果继续保持 `isError=false`，不会被 typed output schema 的校验错误替代。`download_random_from_recommendation` 在成功和失败时都不附加 ImageContent，即使请求了 `delivery=image_content`。

## 作品与用户读取

| tool | 参数 | structured output |
| --- | --- | --- |
| `search_illust` | `word`、`search_target`、`sort`、`duration`、`offset`、`search_r18`、`include_thumbnail` | 作品列表。 |
| `illust_detail` | `illust_id` | 作品详情。 |
| `illust_related` | `illust_id`、`offset`、`include_thumbnail` | 相关作品。 |
| `illust_ranking` | `mode`、`date`、`offset`、`include_thumbnail` | 排行榜作品。 |
| `illust_recommended` | `offset`、`include_thumbnail` | 兼容旧推荐作品 tool；保留既有文本输出，但经公开 SDK 调用链执行。 |
| `recommended` | 必填 `kind`（`all`、`illust`、`manga`、`novel`、`user`），可选 `page`、`limit` | 通过认证 App SDK 返回 `{kind, illusts, manga, novels, user_previews, pagination}`；单类只填对应流，`all` 顺序读取四流。每条流独立应用分页，`pagination` 按流给出逻辑页信息；不暴露 SDK cursor，不支持 Web fallback。 |
| `trending_tags_illust` | 无 | 热门标签。 |
| `illust_follow` | `restrict`、`offset`、`include_thumbnail` | 关注新作；需要认证。 |
| `search_user` | `word`、`offset` | 用户列表；匿名 fallback 是相关作者去重，不是官方用户名搜索。 |
| `get_thumbnail_base64` | `illust_id` | `data:image/jpeg;base64,...`。 |
| `user_detail` | 必填 `user_id` | 完整稳定的 `{user, profile, profile_publicity, workspace}`；需要认证，不支持 Web fallback。 |
| `user_artworks` | 可选 `user_id`、`type`、`page`、`limit` | `{user_id, items, pagination}`；缺省 UID 为当前认证用户。 |
| `user_bookmarks` | 可选 `user_id`、旧 alias `user_id_to_check`、`restrict`、`tag`、`page`、`limit`、废弃 `max_bookmark_id` | `{user_id, items, pagination}`；缺省 UID 为当前认证用户。 |
| `user_following` | 可选 `user_id`、旧 alias `user_id_to_check`、`restrict`、`page`、`limit`、废弃 `offset` | `{user_id, items, pagination}`；缺省 UID 为当前认证用户。 |

## 写操作

| tool | 参数 | structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`、可选 `restrict`、`tags` | `{success, action, illust_id}`。 |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}`。 |
| `follow_user` | `user_id`、可选 `restrict` | `{success, action, user_id}`。 |
| `unfollow_user` | `user_id` | `{success, action, user_id}`。 |

表中写操作均走 SDK 路径、需要认证；失败 `success=false` 且 MCP result 为 `isError=true`。
