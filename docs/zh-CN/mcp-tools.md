# MCP 工具

[English](../en/mcp-tools.md) | 简体中文 | [日本語](../ja/mcp-tools.md) | [文档索引](../index.md)

以 `pixiv mcp` 启动 stdio server。stdout 仅用于 JSON-RPC；操作日志写入用户主目录 `~/.pixiv-cli/logs`（Windows 为 `%USERPROFILE%\.pixiv-cli\logs`）下的按日纯文本文件 `YYYY-MM-DD.txt`（默认保留 7 天），终端默认无日志痕迹。MCP 不提供 HTTP endpoint。

有 refresh token 时 App API 为主路径，失败不自动回落 Web；无 refresh token 且 `web_fallback_enabled=true` 时，仅匿名白名单读 tool 可用 Web API。查询型 tool（包括作品查询）同时返回紧凑文本摘要与对应的 typed structured output；其可分类失败会令 result `isError=true`，保留安全错误文本和对应 structured output。其他文本型 MCP tool 的失败保留既有 Content、structured output、文本和 `isError=false` wire 形式，但对应文件日志 operation event 会使用 error level 和 `result=error`；事件只保留 operation、稳定 SDK 分类、backend/status 及安全 ID，不记录原始错误文本、tool 输入、query、token、Cookie、URL、path 或 response body。公开可写的未知 SDK error code 不进入事件，未知 backend 归类为 `local`，不会回显原值。正常空结果仍记录为成功。

## 分页

新 SDK 列表 tool 均使用：

- `limit`：最大项目数；`0` 表示跟随上游直到没有下一批；不传时读取一个上游批次。
- `page`：从 1 开始的逻辑页，必须配正数 `limit`。
- 输出 `pagination.page`、`limit`、`returned`、`has_more`、可选 `next_page`。

SDK cursor 不出现在 MCP 参数或输出。列表工具统一使用逻辑 `page`/`limit`。

## 配置、认证与下载

| tool | 参数 | structured output |
| --- | --- | --- |
| `set_download_path` | `path` | 文本状态。 |
| `refresh_token` | 无 | 当前认证账号摘要。 |
| `set_refresh_token` | 原始 App API `refresh_token` | 当前会话认证结果；不写 `auth.json`；Cookie 输入会被拒绝。 |
| `download` | `illust_id`、`illust_ids` 和/或 `urls`，可选 `pages`/`quality`；`delivery` 仅 `local_path` | `{items, failures}` 下载报告；每个成功项含规范作品 URL、ID、类型和本地文件页号/路径；不内嵌图片内容。 |
| `download_random_from_recommendation` | 可选 `count`（省略或 `null` 时默认 5；显式值须为 1..20），可选 `pages`/`quality`；`delivery` 仅 `local_path` | 下载结果文本与 structured 本地文件元数据；不内嵌图片内容。 |

`refresh_token` 在 SDK/config/proxy 初始化失败时不会误报“未设置 refresh token”：context 取消与 deadline 保留明确文案，公开 `*pixiv.Error` 保留安全 code/operation/backend 分类，其他未知初始化错误不回显原始细节。真正执行 refresh 时，仅 `unauthorized` 保留缺少 token 提示；未知执行错误同样返回脱敏排查提示。该 tool 的 wire 返回 `isError=false`，真实失败通过前述文件日志 event 可观测。

`download_random_from_recommendation.count` 限制本次请求的作品数，不限制一个作品展开的文件数。显式传入 0、负数或大于 20 的值会返回参数错误，不会改写为默认值或边界值；推荐列表少于请求数时则下载列表中实际可用的作品。该 tool 与 `download` 一样只返回下载结果文本与 structured 本地文件元数据，不内嵌图片内容。

以 HTTP(S) 代理启动 MCP server 时，其媒体资源下载会刻意使用 HTTP/1.1。App API、OAuth 与 Web 元数据请求仍保留常规协议协商；此行为规避部分代理特有的 HTTP/2 流重置，不改变认证或所选下载质量。

`download.urls` 只在本地识别稳定的官方 Pixiv HTTPS 页面：`pixiv.net` 或 `www.pixiv.net` 的 `/artworks/{id}`、`/users/{id}`、`/users/{id}/artworks`，可带 locale、query 和 fragment。解析不跟随跳转、不抓取 HTML；短链、旧式 URL、小说、FANBOX、Pixivision、Sketch、其他 host 或路径都会在联网和打开下载器前被拒绝，安全错误文本不会回显原始 URL。作品 URL 下载一件作品；用户 URL 按输入位置展开其全部 `illust`、`manga`、`ugoira`，不下载小说，并且必须使用 App OAuth，不能走匿名 Web fallback。

下载先按 `illust_ids` 的数组顺序、再按可选 `illust_id`、最后按 `urls` 的数组顺序处理；MCP 的独立字段不能表达它们之间的交错顺序。不会跨次持久化、去重、缓存或补齐历史记录，重复引用会再次处理。用户批量下载没有隐式数量、分页、重试或超时限制。取消立即停止；单件或单页失败不会阻止之后的目标，`items` 与 `failures` 会一起返回，因此含失败的报告 result 为 `isError=true`。成功项使用规范作品 URL，失败项给出安全的 URL/ID/类型/消息摘要。

两个下载 tool 在参数校验、SDK、推荐获取、下载、结果整理失败时，都会保留原有业务错误文本，并返回有效 structured output：`delivery` 固定为 `local_path`（非法 `delivery` 时同样回落到该值），`items`、`failures` 与 `files` 是空数组而不是 `null`。这些全量失败结果返回 `isError=false`，不会被 typed output schema 的校验错误替代。成功与失败均不内嵌图片内容。

## 作品与用户读取

| tool | 参数 | structured output |
| --- | --- | --- |
| `search_illust` | `word`、`search_target`、`sort`、`duration`、`start_date`、`end_date`、`page`、`limit`、`rating`、`content_type`、`ai_mode`、`aspect_ratio`、`resolution`、`tool`、`bookmark_min`、`bookmark_max` | `{items, pagination, text}`；`items` 可直接作为后续下载的作品引用来源。 |
| `search_novel` | `word`、`search_target`、`sort`、`duration`、`page`、`limit`、`rating`、`min_text_length`、`max_text_length`、`original_only` | 仅 App 的 `{novels, pagination, text}`；可分类失败会令 `isError=true`。 |
| `search_illust_options` | 必填 `word` | 当前搜索词可用的 `{tools,text}`；需要认证，不支持 Web fallback。 |
| `illust_detail` | `illust_id` 或 `url` 二选一 | `{illust, text}`；作品详情；Pixiv 提供时包含原始 HTML `caption`。 |
| `illust_related` | `illust_id`、`page`、`limit` | `{items, pagination, text}` 相关作品。 |
| `illust_ranking` | `mode`、`date`、`page`、`limit` | `{items, pagination, text}` 排行榜作品。 |
| `illust_recommended` | `page`、`limit` | `{items, pagination, text}` 推荐作品；文本输出经公开 SDK 调用链执行。 |
| `recommended` | 必填 `kind`（`all`、`illust`、`manga`、`novel`、`user`），可选 `page`、`limit` | 通过认证 App SDK 返回 `{kind, illusts, manga, novels, user_previews, pagination}`；单类只填对应流，`all` 顺序读取四流。每条流独立应用分页，`pagination` 按流给出逻辑页信息；不暴露 SDK cursor，不支持 Web fallback。 |
| `trending_tags_illust` | 无 | `{tags, text}` 热门标签。 |
| `illust_follow` | `restrict`、`page`、`limit` | `{items, pagination, text}` 关注新作；需要认证。 |
| `search_user` | `word`、`page`、`limit` | `{source, user_previews, pagination, text}`；认证官方 App 搜索为 `app_search`，匿名 fallback 为 `related_illust_authors`，后者不是用户名搜索。 |
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
- `search_target`：`partial_match_for_tags|exact_match_for_tags|title_and_caption|keyword`；`keyword` 搜索标签、标题、说明文字，且需要 App OAuth。
- `duration`：`within_last_day|within_last_week|within_last_month|within_half_year|within_year`；不能和日期边界同用。两个长周期会在本地展开为包含边界的东京日期区间。
- `start_date` / `end_date`：包含边界的 `YYYY-MM-DD`；可只给一端，两端都有时起始不得晚于结束。
- `bookmark_min` / `bookmark_max`：包含边界的非负公开收藏数；最小值不能大于最大值，且都需要 App OAuth 和有效的 Pixiv 高级会员。

有 refresh token 时，分辨率、横纵比、工具、作品类型和 `ai_mode=exclude` 由 App 服务端筛选，
`rating` 与 `ai_mode=only` 由 public SDK 基于 App 返回字段筛选；App 失败不回落 Web。无 token 的匿名
Web 路径只执行已验证可靠的筛选；`rating=r18|r18g|mature`、`search_target=keyword` 和收藏数边界在请求前返回需要登录，不伪装成空结果。Pixiv 对收藏数边界还要求高级会员。
`search_illust_options` 只走 App API。所有搜索 tool 都不接受 Cookie。

对认证态 `search_illust`，标签 `search_target` 会在 `word` 中保留已验证的 Pixiv App 查询语法：
`exact_match_for_tags` 下 `tagA tagB` 要求两个完整标签同时存在，大写 `tagA OR tagB` 接受任一标签；字面量
`AND` 不是运算符。`partial_match_for_tags` 也接受已验证的大写 `OR`，但它使用模糊标签词，不能视为严格的
精确标签 AND。`title_and_caption` 和 `keyword` 都没有布尔标签契约，且尚未验证字面量大写 `OR` 标签/关键词的转义语法。

`search_novel.rating` 使用 `all|sfw|r18|r18g|mature`。`min_text_length`、`max_text_length` 为非负字符数边界，
`0` 关闭对应边界；非零上界小于下界会返回参数错误。`original_only` 仅保留标记为原创的小说。Pixiv 没有已验证的
这三项 App wire 参数，因此 public SDK 按每条结果的 `x_restrict`、`text_length`、`is_original` 验证；字段缺失是
上游错误，不会静默视为不匹配。`search_novel` 本身只支持 App。

`search_user` 同时返回文本内容与 structured `source`、`user_previews`、`pagination`。匿名 fallback 的固定英文
文本会明确说明它返回的是相关插画作者，不是官方用户名搜索。

MCP 的固定状态、错误、列表标题、字段标签和排行榜文本均使用英文；Pixiv 上游返回的作品元数据和 tool 参数保持原文。作品列表的 MCP 文本按上游顺序完整列出每个作品的全部 tags，不做前 5 项截断；SDK tool 的 structured output schema 和内容保持不变。`illust_ranking` 对已知 mode 使用稳定英文标题：`day`、`day_male`、`day_female`、`week`、`week_original`、`week_rookie`、`month` 分别为 `Daily ranking`、`Daily ranking (male)`、`Daily ranking (female)`、`Weekly ranking`、`Weekly original ranking`、`Weekly rookie ranking`、`Monthly ranking`；`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、`day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g` 分别为 `Daily manga ranking`、`Weekly manga ranking`、`Monthly manga ranking`、`Weekly rookie manga ranking`、`Daily R-18 ranking`、`Daily male R-18 ranking`、`Daily female R-18 ranking`、`Weekly R-18 ranking`、`Weekly R-18G ranking`。最后九种需要 App 认证；无认证时明确返回认证错误，不会代换为匿名日榜。未来 mode 在上游成功时显示原 mode 后接 `ranking`。

## 写操作

| tool | 参数 | structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`、可选 `restrict`、`tags` | `{success, action, illust_id}`。 |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}`。 |
| `follow_user` | `user_id`、可选 `restrict` | `{success, action, user_id}`。 |
| `unfollow_user` | `user_id` | `{success, action, user_id}`。 |

表中写操作均走 SDK 路径、需要认证；失败 `success=false` 且 MCP result 为 `isError=true`。
