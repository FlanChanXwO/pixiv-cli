# MCP 工具

[English](../en/mcp-tools.md) | 简体中文 | [文档索引](../index.md)

通过 `pixiv mcp` 启动 stdio server。启动前使用 CLI 配置账号与本地下载设置；运行时 MCP 按配置使用已选本地账号或 `PIXIV_REFRESH_TOKEN`。

所有实体读取 tool 返回 structured `{records, pagination?}`。每条 Record 都有稳定的 `id`、`type`、`url`，并保留适用的公开 SDK 字段；text Content 只提供同一操作的简短摘要。

## 错误与分页

不符合 MCP schema 的输入会产生 JSON-RPC schema error。tool 开始执行后的任何失败都会保留其约定的 structured 结果并设置 `isError=true`：下载保留下载报告形状，实体读取保留 `records: []`。正常空结果仍是成功。

列表 tool 使用 `limit` 与 `page`：

- 省略 `limit` 时读取一个上游批次；`limit: 0` 持续读取至上游耗尽。
- `page` 是从 1 开始的逻辑页，必须配合正数 `limit`。
- 筛选先于逻辑分页；以 `type + id` 去重。服务端会继续读取上游页，直至填满所请求逻辑页或上游耗尽。

SDK cursor 保持在服务端内部。列表结果提供 `pagination.page`、`limit`、`returned`、`has_more`，以及适用时的 `next_page`。

## 实体筛选

实体列表可使用按实体命名的可选 filter：

| Filter | 字段 |
| --- | --- |
| `illust_filter` | `id`（正数）、`type`（`illust`、`manga` 或 `ugoira`）、`tags`（全部精确匹配）、`min_views`（非负）、`min_pages`（非负） |
| `novel_filter` | `id`（正数）、`tags`（全部精确匹配）、`min_views`（非负） |
| `user_filter` | `id`（正数） |

插画列表接收 `illust_filter`，小说列表接收 `novel_filter`，用户列表接收 `user_filter`；混合推荐可分别为每种实体提供对应 filter。所有插画列表 tool 和 `download` 还接收顶层 `filter` 字符串：它是基于公开插画字段的安全本地表达式，并与 `illust_filter` 按 AND 组合。表达式只支持比较、`and`/`or`/`not`、`in`/`not in`、数组以及 `any`/`all`；例如 `bookmarkCount >= 5000 and xRestrict == 0`、`any(tags, # in ["miku"])`。混合结果使用 `filter` 时会丢弃小说与用户记录。

## 下载

| tool | 参数 | structured output |
| --- | --- | --- |
| `download` | `src` 与有序 `srcs` 二选一；每项为 PID、支持的 Pixiv 作品/用户/公开收藏/插画系列 URL 或允许的 CDN URL。可选 `pages`（包括 `3-`）、`quality`、`concurrency`、`ugoira_mode`（`gif`、`apng`、`zip`、`frames`）、`filter`、`archive`、`directory_template`、`write_metadata`、`retries`、`retry_delay` 与 `delivery: "local_path"`。 | `{items, failures, files, text}` 本地文件报告。 |
| `download_random_from_recommendation` | 可选 `count`（省略或 `null` 为 5；显式值为 1..20）、可选 `pages`、`quality`、`ugoira_mode` 与 `delivery: "local_path"`。 | 使用同一错误语义的本地文件报告。 |

用户、公开收藏和插画系列 URL 会按来源顺序展开认证态的插画、漫画和 ugoira，重复 artwork ID 只下载一次。`filter` 在取得作品详情后、写文件前执行；CDN URL 没有作品元数据，因此会拒绝 `filter`。`archive` 是 SQLite 文件，只有全部选中产物与要求的 metadata sidecar 都成功后才记录 artwork。目录模板和文件名模板都支持 `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}`。`concurrency: 0` 使用 `2 × GOMAXPROCS`。资源请求默认重试三次（1/2/4 秒；有效 `Retry-After` 优先），资源缓存会安全续传 validator 匹配的残片。某一项失败会保留在 `failures` 中，其他独立项继续；任一失败都会设置 `isError=true`。

## 读取

| tool | 参数 |
| --- | --- |
| `search_illust` | `word`、搜索筛选、`page`、`limit`、可选 `filter` 与 `illust_filter` |
| `search_novel` | `word`、小说搜索筛选、`page`、`limit`、可选 `novel_filter` |
| `illust_detail` | `illust_id` 或支持的 `url` 二选一 |
| `illust_related`、`illust_ranking`、`illust_recommended` | 对应操作参数、`page`、`limit`、可选 `filter` 与 `illust_filter` |
| `recommended` | 必填 `kind`（`all`、`illust`、`manga`、`novel`、`user`）、`page`、`limit`、可选 `filter` 与适用的实体 filter |
| `trending_tags_illust` | 无参数，返回 `{tags, text}` |
| `timeline_illust_following` | `restrict`、`page`、`limit`、可选 `filter` 与 `illust_filter` |
| `timeline_novel_following` | `restrict`、`page`、`limit`、可选 `novel_filter` |
| `timeline_illust_latest` | 必填 `content_type`（`illust` 或 `manga`）、`page`、`limit`、可选 `filter` 与 `illust_filter` |
| `timeline_novel_latest` | `page`、`limit`、可选 `novel_filter` |
| `mypixiv_users` | `page`、`limit`、可选 `user_filter` |
| `mypixiv_illusts` / `mypixiv_novels` | `page`、`limit`、插画可选顶层 `filter`，以及对应的插画或小说 filter |
| `search_user` | `word`、`page`、`limit`、可选 `user_filter` |
| `user_detail` | 必填 `user_id` |
| `user_artworks`、`user_novels`、`user_bookmarks`、`user_following` | 可选 `user_id`、操作专有参数、`page`、`limit`、插画列表可选顶层 `filter`，以及对应实体 filter |

`search_illust.tool` 使用 [CLI 参考](cli-reference.md#drawing-tool-catalog)中的版本化绘图工具目录，必须精确匹配。唯一的单编辑拼写修正会在参数错误中给出建议；含混前缀会直接报错。

所有读取都要求有效的 access token，并走 Pixiv App API。不存在匿名或 Web fallback；已删除的 `web_fallback_enabled` 配置若仍存在会返回 `removed_setting`。

## 写操作

| tool | 参数 | structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`、可选 `restrict`、`tags` | `{success, action, illust_id}` |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `follow_user` | `user_id`、可选 `restrict` | `{success, action, user_id}` |
| `unfollow_user` | `user_id` | `{success, action, user_id}` |

写操作失败时 `success=false` 且 MCP result 为 `isError=true`。

## FANBOX MCP server

使用 `pixiv fanbox mcp` 启动独立的只读 server。它使用选定的本地 FANBOX 账号，session 值不会进入 tool
输入或输出。注册的 tool 为：

| Tool | 作用 |
| --- | --- |
| `fanbox_current_user` | 验证当前 FANBOX session 并返回安全 identity 摘要。 |
| `fanbox_creator`、`fanbox_creators` | 读取一个 creator profile 或 supporting/following creator。 |
| `fanbox_creator_tags` | 读取 creator tag。 |
| `fanbox_creator_posts`、`fanbox_tagged_posts` | 使用 SDK cursor 读取 creator/tag 帖子。 |
| `fanbox_post`、`fanbox_home`、`fanbox_supporting` | 读取单个帖子或 feed。 |
| `fanbox_resolve_url` | 将支持的 FANBOX 页面 URL 解析为 typed reference。 |
| `fanbox_open_resource` | 校验并打开 FANBOX media reference，只返回 status/header，不返回 bytes。 |

Pixiv 与 FANBOX 使用分离的 server，不交叉 credential、proxy 或 route。FANBOX native `--proxy`/`--no-proxy`
只影响 native FANBOX 请求；可选的 FlareSolverr service 与其 upstream proxy 保持独立。

## Debug 与 stdout

`pixiv --debug mcp` 与 `pixiv --debug fanbox mcp` 只向 stderr 写 typed、安全的生命周期、网络、challenge、
solver、下载与失败诊断。MCP stdout 始终是纯 JSON-RPC，tool schema 和 structured failure 不变；两个 server
分别维护本地 request number。不会输出 raw URL query、Cookie、token、proxy userinfo 或 FlareSolverr clearance。
