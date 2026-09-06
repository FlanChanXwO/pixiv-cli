# Command-to-upstream contract matrix

观测日期：2026-09-05（Asia/Shanghai）。

说明：

- `P1/P2` 是两页 item count。
- `—` 表示本次不要求或未测试。
- `pagination_exempt` 是用户确认的数据受限例外。
- `confirmed` 是当前 evidence verdict；完整 contract 以 `contract_frozen` 为准。

| Case | 命令 | Method | Path | 参数 | P1 | P2 | Continuation | Adapter | SDK | Verdict | 备注 |
| --- | --- | --- | --- | --- | ---: | ---: | --- | --- | --- | --- | --- |
| novel-detail-v1 | detail novel | GET | /v1/novel/detail | novel_id | — | — | none | — | — | rejected | live v1 不可用 |
| novel-detail-v2 | detail novel | GET | /v2/novel/detail | novel_id | — | — | none | not_tested | not_tested | not_tested | upstream migration-ready；public 未验证 |
| novel-series-v1 | series novel | GET | /v1/novel/series | series_id | 28 | — | none | — | — | rejected | required detail 缺失 |
| novel-series-v2 | series novel | GET | /v2/novel/series | series_id,last_order | — | — | last_order | — | — | inconclusive | 生产仍为 v1 |
| novel-content-app | detail novel --content | GET | /v1/novel/content | novel_id | — | — | none | — | — | rejected | live v1 不可用 |
| novel-content-webview | detail novel --content | GET | /webview/v2/novel | id | — | — | none | — | — | not_tested | 排除 |
| novel-new | timeline latest novel | GET | /v1/novel/new | filter,max_novel_id | 30 | 30 | max_novel_id | rejected | inconclusive | inconclusive | 当前使用 offset |
| novel-follow | timeline following novel | GET | /v1/novel/follow | restrict | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| novel-recommended | recommended novel | GET | /v1/novel/recommended | — | 33 | 33 | offset | confirmed | confirmed | confirmed | |
| novel-ranking | candidate novel ranking | GET | /v1/novel/ranking | filter,mode | 30 | 30 | offset | not_tested | not_tested | not_tested | production owner missing |
| user-novels | user novels | GET | /v1/user/novels | filter,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-novel-bookmarks-public | bookmark novel public | GET | /v1/user/bookmarks/novel | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-novel-bookmarks-private | bookmark novel private | GET | /v1/user/bookmarks/novel | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | private data verified |
| novel-comments-v2 | comment novel | GET | /v2/novel/comments | novel_id,offset | — | — | offset | confirmed | confirmed | inconclusive | access-control risk |
| novel-comments-v3 | comment novel candidate | GET | /v3/novel/comments | novel_id,offset | — | — | offset | candidate | candidate | inconclusive | candidate fixture passed |
| search-illust-all | search artwork | GET | /v1/search/illust | filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-illust | search artwork illust | GET | /v1/search/illust | content_type,filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-manga | search artwork manga | GET | /v1/search/illust | content_type,filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-ugoira | search artwork ugoira | GET | /v1/search/illust | content_type,filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-rating | search artwork rating | GET | /v1/search/illust | x_restrict | — | — | — | rejected | rejected | rejected | server ignores parameter |
| illust-recommended | recommended artwork | GET | /v1/illust/recommended | filter,continuation state | 83 | — | offset+state | confirmed | confirmed | inconclusive | second page error |
| illust-new | timeline latest artwork | GET | /v1/illust/new | content_type,filter | 30 | 30 | max_illust_id | confirmed | confirmed | confirmed | |
| illust-ranking | ranking artwork | GET | /v1/illust/ranking | mode | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| user-illusts-illust | user artworks illust | GET | /v1/user/illusts | filter,type,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-illusts-manga | user artworks manga | GET | /v1/user/illusts | filter,type,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-illust-bookmarks-public | bookmark artwork public | GET | /v1/user/bookmarks/illust | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-illust-bookmarks-private | bookmark artwork private | GET | /v1/user/bookmarks/illust | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | private data verified |
| illust-comments-v3 | comment artwork | GET | /v3/illust/comments | illust_id,offset | — | — | offset | rejected | rejected | rejected | live date/access-control mismatch |
| ugoira-metadata | ugoira metadata | GET | /v1/ugoira/metadata | illust_id | 1 | — | none | confirmed | confirmed | confirmed | |
| stamps | candidate stamps | GET | /v1/stamps | — | 40 | — | none | not_tested | not_tested | not_tested | production owner missing |
| illust-comment-text | comment artwork text | POST | /v1/illust/comment/add | illust_id,comment | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| illust-comment-reply | reply artwork comment | POST | /v1/illust/comment/add | illust_id,comment,parent_comment_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| novel-comment-stamp | comment novel stamp | POST | /v1/novel/comment/add | novel_id,comment,stamp_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| novel-comment-text | comment novel text | POST | /v1/novel/comment/add | novel_id,comment | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| illust-comment-delete | delete artwork comment | POST | /v1/illust/comment/delete | comment_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| novel-comment-delete | delete novel comment | POST | /v1/novel/comment/delete | comment_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |

## 迁移准入规则

本矩阵的最终 verdict 表示 `public_ready`。

开始 TDD 生产实现前，还要读取 `api-migration-verification.md` 的 `migration_ready`。

规则：

- `migration_ready`：允许开始 endpoint adapter / SDK 实现。
- `confirmed` 或 `confirmed / pagination_exempt`：允许公开 CLI / MCP / docs surface。
- `inconclusive`、`rejected`、`not_tested`：不能公开。
- 没有 `migration_ready`：不能修改生产 endpoint。


## 原始计划中尚未进入 strict manifest 的候选

| Case | Method | Path | 参数/语义 | Wire | Response | Pagination | Adapter | SDK | 状态 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| search-novel-period | GET | /v1/search/novel | start_date,end_date | not_tested | not_tested | not_tested | not_tested | not_tested | 必须补测 |
| novel-bookmark-tags | GET | /v1/user/bookmark-tags/novel | restrict,user_id | not_tested | not_tested | pagination_exempt | not_tested | not_tested | 必须补测 |
| novel-bookmark-detail | GET | /v2/novel/bookmark/detail | novel_id | not_tested | not_tested | none | not_tested | not_tested | 必须补测 |
| novel-bookmark-add | POST | /v2/novel/bookmark/add | novel_id,restrict,tags | not_tested | not_tested | read_back | not_tested | not_tested | 必须补测 |
| novel-bookmark-delete | POST | /v1/novel/bookmark/delete | novel_id | not_tested | not_tested | read_back | not_tested | not_tested | 必须补测 |
| illust-bookmark-tags | GET | /v1/user/bookmark-tags/illust | restrict,user_id | not_tested | not_tested | pagination_exempt | not_tested | not_tested | 必须补测 |
| illust-bookmark-subtype | GET | /v1/user/bookmarks/illust | type/content_type | not_tested | not_tested | logical | not_tested | not_tested | 必须补测 |
| illust-recommended-subtype | GET | /v1/illust/recommended | content_type | not_tested | not_tested | not_tested | not_tested | not_tested | 必须补测 |
| illust-new-subtype-expansion | GET | /v1/illust/new | manga/ugoira/compound | partial | partial | partial | partial | partial | 必须补测 |
| novel-comment-total | GET | /v2或v3/novel/comments | total_comments/include_total | partial | inconclusive | inconclusive | partial | partial | 必须补测 |
| illust-comment-total | GET | /v3/illust/comments | total_comments/include_total | partial | inconclusive | inconclusive | partial | partial | 必须补测 |


## Goal-3 状态语义

本矩阵的历史 verdict 用于描述 evidence、fixture 与当前生产覆盖，不等同于 capability 不存在。Goal-3 内按 `contract_frozen`、`migration_ready`、`public_ready` 逐层推进；`inconclusive` / `not_tested` 需要补 snapshot 或实现证据，但不产生新的 Goal。
