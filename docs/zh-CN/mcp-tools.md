# MCP 工具

[English](../en/mcp-tools.md) | 简体中文 | [文档索引](../index.md)

以 `pixiv mcp` 启动 stdio server。stdout 仅用于 JSON-RPC；操作日志写入用户 state 目录下 `pixiv/logs` 的按日 JSONL（默认保留 7 天），终端默认无日志痕迹。MCP 不提供 HTTP endpoint。

有 refresh token 时 App API 为主路径，失败不自动回落 Web；无 refresh token 且 `web_fallback_enabled=true` 时，仅匿名白名单读 tool 可用 Web API。SDK 路径的用户详情、用户列表和收藏/关注写操作同时返回文本内容与 structured output；其可分类失败会令 result `isError=true`，保留安全错误文本和对应 structured output。遗留 MCP tool 的失败继续保持既有 Content、structured output、文本和 `isError=false` wire 兼容，但对应文件日志 operation event 会使用 error level 和 `result=error`；事件只保留 operation、稳定 SDK 分类、backend/status 及安全 ID，不记录原始错误文本、tool 输入、query、token、Cookie、URL、path 或 response body。公开可写的未知 SDK error code 不进入事件，未知 backend 归类为 `local`，不会回显原值。正常空结果仍记录为成功。

## 分页

新 SDK 列表 tool 均使用：

- `limit`：最大项目数；`0` 表示跟随上游直到没有下一批；不传时兼容为一个上游批次。
- `page`：从 1 开始的逻辑页，必须配正数 `limit`。
- 输出 `pagination.page`、`limit`、`returned`、`has_more`、可选 `next_page`。

SDK cursor 不出现在 MCP 参数或输出。列表工具统一使用逻辑 `page`/`limit`。

## 配置、认证与下载

| tool | 参数 | structured output |
| --- | --- | --- |
| `set_download_path` | `path` | 文本状态。 |
| `refresh_token` | 无 | 当前认证账号摘要。 |
| `set_refresh_token` | 原始 App API `refresh_token` | 当前会话认证结果；不写 `auth.json`；Cookie 输入会被拒绝。 |
| `download` | `illust_id` 或 `illust_ids`，可选 `pages`/`quality`；`delivery` 仅 `local_path` | 本地文件 path/file_uri/mime_type/页号/大小；不内嵌图片内容。 |
| `download_random_from_recommendation` | 可选 `count`（省略或 `null` 时默认 5；显式值须为 1..20），可选 `pages`/`quality`；`delivery` 仅 `local_path` | 下载结果文本与 structured 本地文件元数据；不内嵌图片内容。 |

`refresh_token` 在 SDK/config/proxy 初始化失败时不会误报“未设置 refresh token”：context 取消与 deadline 保留明确文案，公开 `*pixiv.Error` 保留安全 code/operation/backend 分类，其他未知初始化错误不回显原始细节。真正执行 refresh 时，仅 `unauthorized` 保留缺少 token 提示；未知执行错误同样返回脱敏排查提示。该 legacy tool 的 wire 仍保持 `isError=false`，真实失败通过前述文件日志 event 可观测。

`download_random_from_recommendation.count` 限制本次请求的作品数，不限制一个作品展开的文件数。显式传入 0、负数或大于 20 的值会返回参数错误，不会改写为默认值或边界值；推荐列表少于请求数时则下载列表中实际可用的作品。该 tool 与 `download` 一样只返回下载结果文本与 structured 本地文件元数据，不内嵌图片内容。

两个下载 tool 在参数校验、SDK、推荐获取、下载、结果整理失败时，都会保留原有业务错误文本，并返回有效 structured output：`delivery` 固定为 `local_path`（非法 `delivery` 时同样回落到该值），`items` 与 `files` 是空数组而不是 `null`。这些遗留失败结果继续保持 `isError=false`，不会被 typed output schema 的校验错误替代。成功与失败均不内嵌图片内容。

## 作品与用户读取

| tool | 参数 | structured output |
| --- | --- | --- |
| `search_illust` | `word`、`search_target`、`sort`、`duration`、`page`、`limit`、`rating`、`content_type`、`ai_mode`、`aspect_ratio`、`resolution`、`tool` | Legacy structured output `{text}`；作品列表仍在文本中呈现。 |
| `search_illust_options` | 必填 `word` | 当前搜索词可用的 `{tools,text}`；需要认证，不支持 Web fallback。 |
| `illust_detail` | `illust_id` | 作品详情。 |
| `illust_related` | `illust_id`、`page`、`limit` | 相关作品。 |
| `illust_ranking` | `mode`、`date`、`page`、`limit` | 排行榜作品。 |
| `illust_recommended` | `page`、`limit` | 推荐作品；文本输出经公开 SDK 调用链执行。 |
| `recommended` | 必填 `kind`（`all`、`illust`、`manga`、`novel`、`user`），可选 `page`、`limit` | 通过认证 App SDK 返回 `{kind, illusts, manga, novels, user_previews, pagination}`；单类只填对应流，`all` 顺序读取四流。每条流独立应用分页，`pagination` 按流给出逻辑页信息；不暴露 SDK cursor，不支持 Web fallback。 |
| `trending_tags_illust` | 无 | 热门标签。 |
| `illust_follow` | `restrict`、`page`、`limit` | 关注新作；需要认证。 |
| `search_user` | `word`、`page`、`limit` | 用户列表；匿名 fallback 是相关作者去重，不是官方用户名搜索。 |
| `user_detail` | 必填 `user_id` | 完整稳定的 `{user, profile, profile_publicity, workspace}`；需要认证，不支持 Web fallback。 |
| `user_artworks` | 可选 `user_id`、`type`、`page`、`limit` | `{user_id, items, pagination}`；缺省 UID 为当前认证用户。 |
| `user_bookmarks` | 可选 `user_id`、`restrict`、`tag`、`page`、`limit` | `{user_id, items, pagination}`；缺省 UID 为当前认证用户。 |
| `user_following` | 可选 `user_id`、`restrict`、`page`、`limit` | `{user_id, items, pagination}`；缺省 UID 为当前认证用户。 |

`search_illust` 的筛选枚举为：

- `rating`：`all|sfw|r18|r18g|mature`；
- `content_type`：`all|illust-and-ugoira|illust|manga|ugoira`；
- `ai_mode`：`all|exclude|only`，其中 Pixiv `AIType==2` 才表示 AI 生成；
- `aspect_ratio`：`all|landscape|portrait|square`；
- `resolution`：`all|high|medium|low`，三档分别为宽高均 `>=3000`、均在 `1000..2999`、均
  `<=999`；
- `tool`：上游绘图工具原值，不做模糊匹配。

有 refresh token 时，分辨率、横纵比、工具、作品类型和 `ai_mode=exclude` 由 App 服务端筛选，
`rating` 与 `ai_mode=only` 由 public SDK 基于 App 返回字段筛选；App 失败不回落 Web。无 token 的匿名
Web 路径只执行已验证可靠的筛选；`rating=r18|r18g|mature` 在请求前返回需要登录，不伪装成空结果。
`search_illust_options` 只走 App API。两项搜索 tool 都不接受 Cookie，当前也不提供收藏数筛选。

作品列表的 MCP 文本按上游顺序完整列出每个作品的全部 tags，不做前 5 项截断；SDK tool 的 structured output schema 和内容保持不变。`illust_ranking` 对已知 mode 使用稳定中文标题：`day`、`day_male`、`day_female`、`week`、`week_original`、`week_rookie`、`month` 分别显示为“每日排行榜”“男性向每日排行榜”“女性向每日排行榜”“每周排行榜”“原创作品排行榜”“新人排行榜”“每月排行榜”；`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、`day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g` 分别显示为“漫画每日排行榜”“漫画每周排行榜”“漫画每月排行榜”“漫画新人排行榜”“R-18 每日排行榜”“男性向 R-18 每日排行榜”“女性向 R-18 每日排行榜”“R-18 每周排行榜”“R-18G 每周排行榜”。最后九种需要 App 认证；无认证时明确返回认证错误，不会代换为匿名日榜。未来 mode 在上游成功时显示原 mode 后接“排行榜”。

## 写操作

| tool | 参数 | structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`、可选 `restrict`、`tags` | `{success, action, illust_id}`。 |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}`。 |
| `follow_user` | `user_id`、可选 `restrict` | `{success, action, user_id}`。 |
| `unfollow_user` | `user_id` | `{success, action, user_id}`。 |

表中写操作均走 SDK 路径、需要认证；失败 `success=false` 且 MCP result 为 `isError=true`。
