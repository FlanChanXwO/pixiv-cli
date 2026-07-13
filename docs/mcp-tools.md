# MCP 工具

以 `pixiv mcp` 启动 stdio server。stdout 仅用于 JSON-RPC，日志写 stderr。MCP 不提供 HTTP endpoint。

有 refresh token 时 App API 为主路径，失败不自动回落 Web；无 refresh token 且 `web_fallback_enabled=true` 时，仅匿名白名单读 tool 可用 Web API。SDK 路径的用户列表和收藏/关注写操作同时返回文本内容与 structured output；其可分类失败会令 result `isError=true`，保留安全错误文本和对应 structured output。遗留 MCP tool 保持既有文本结果兼容，不承诺统一 `isError` 语义。

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
| `set_refresh_token` | `refresh_token` | 当前会话认证结果；不写 `auth.json`。 |
| `download` | `illust_id` 或 `illust_ids`，可选 `delivery` | 下载文件、URI、MIME、大小；`image_content` 另附 ImageContent。 |
| `download_random_from_recommendation` | `count`，可选 `delivery` | 同 `download`。 |

## 作品与用户读取

| tool | 参数 | structured output |
| --- | --- | --- |
| `search_illust` | `word`、`search_target`、`sort`、`duration`、`offset`、`search_r18`、`include_thumbnail` | 作品列表。 |
| `illust_detail` | `illust_id` | 作品详情。 |
| `illust_related` | `illust_id`、`offset`、`include_thumbnail` | 相关作品。 |
| `illust_ranking` | `mode`、`date`、`offset`、`include_thumbnail` | 排行榜作品。 |
| `illust_recommended` | `offset`、`include_thumbnail` | 推荐作品；需要认证。 |
| `trending_tags_illust` | 无 | 热门标签。 |
| `illust_follow` | `restrict`、`offset`、`include_thumbnail` | 关注新作；需要认证。 |
| `search_user` | `word`、`offset` | 用户列表；匿名 fallback 是相关作者去重，不是官方用户名搜索。 |
| `get_thumbnail_base64` | `illust_id` | `data:image/jpeg;base64,...`。 |
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
