# MCP tools

English | [简体中文](../zh-CN/mcp-tools.md) | [日本語](../ja/mcp-tools.md) | [Documentation index](../index.md)

Start the stdio server with `pixiv mcp`. Configure accounts and local download settings with the CLI before starting the server; MCP then uses the selected local account or `PIXIV_REFRESH_TOKEN` according to the runtime configuration.

Every entity-reading tool returns structured `{records, pagination?}`. Each record has stable `id`, `type`, and `url` fields, plus the applicable public SDK fields. Text content is a compact summary of the same operation.

## Errors and pagination

Invalid MCP input is a JSON-RPC schema error. Once a tool begins running, every failure returns its documented structured result together with `isError=true`; downloads retain their report shape, and record reads retain `records: []`. A normal empty result is successful.

List tools accept `limit` and `page`:

- Omit `limit` to read one upstream batch. `limit: 0` follows upstream until it is exhausted.
- `page` is a 1-based logical page and requires a positive `limit`.
- Filtering happens before logical pagination. Duplicate records are removed by `type + id`; the server continues across upstream pages until it fills the requested logical page or reaches the end.

The server keeps SDK cursors internal. List results expose `pagination.page`, `limit`, `returned`, `has_more`, and, where applicable, `next_page`.

## Entity filters

Record-list tools use an optional filter named for the entity they return:

| Filter | Fields |
| --- | --- |
| `illust_filter` | `id` (positive), `type` (`illust`, `manga`, or `ugoira`), `tags` (all exact tags), `min_views` (non-negative), `min_pages` (non-negative) |
| `novel_filter` | `id` (positive), `tags` (all exact tags), `min_views` (non-negative) |
| `user_filter` | `id` (positive) |

An illustration list accepts `illust_filter`, a novel list accepts `novel_filter`, and a user list accepts `user_filter`. Mixed recommendations accept the matching filter for each record kind.

## Downloads

| Tool | Input | Structured output |
| --- | --- | --- |
| `download` | Exactly one of `src` or ordered `srcs`; each source is a PID, supported Pixiv artwork/user URL, or allowed CDN URL. Optional `pages`, `quality`, `concurrency`, `ugoira_format` (`gif` or `apng`), and `delivery: "local_path"`. | `{items, failures, files, text}` with local-file metadata. |
| `download_random_from_recommendation` | Optional `count` (omitted or `null` is 5; explicit values are 1..20), optional `pages` and `quality`, and `delivery: "local_path"`. | Local-file report using the same error semantics. |

User URLs expand authenticated illustration, manga, and ugoira works in source order; duplicate artworks are fetched once. `concurrency: 0` uses `2 × GOMAXPROCS`. The resource cache safely resumes a validator-matched partial file. A failed item remains in `failures` while independent items continue, and any failure sets `isError=true`.

## Reads

| Tool | Input |
| --- | --- |
| `search_illust` | `word`, search filters, `page`, `limit`, optional `illust_filter` |
| `search_novel` | `word`, novel-search filters, `page`, `limit`, optional `novel_filter` |
| `illust_detail` | Exactly one of `illust_id` or supported `url` |
| `illust_related`, `illust_ranking`, `illust_recommended` | Their operation inputs, `page`, `limit`, optional `illust_filter` |
| `recommended` | Required `kind` (`all`, `illust`, `manga`, `novel`, `user`), `page`, `limit`, and applicable entity filters |
| `trending_tags_illust` | No input; returns `{tags, text}` |
| `timeline_illust_following` | `restrict`, `page`, `limit`, optional `illust_filter` |
| `timeline_novel_following` | `restrict`, `page`, `limit`, optional `novel_filter` |
| `timeline_illust_latest` | Required `content_type` (`illust` or `manga`), `page`, `limit`, optional `illust_filter` |
| `timeline_novel_latest` | `page`, `limit`, optional `novel_filter` |
| `mypixiv_users` | `page`, `limit`, optional `user_filter` |
| `mypixiv_illusts` / `mypixiv_novels` | `page`, `limit`, and the matching illustration or novel filter |
| `search_user` | `word`, `page`, `limit`, optional `user_filter` |
| `user_detail` | Required `user_id` |
| `user_artworks`, `user_novels`, `user_bookmarks`, `user_following` | Optional `user_id`, operation-specific inputs, `page`, `limit`, and the matching entity filter |

`search_illust.tool` uses the versioned drawing-tool catalog from the [CLI reference](cli-reference.md#drawing-tool-catalog). It requires an exact value; a unique one-edit spelling correction is offered in the validation error, while ambiguous prefixes are rejected.

Authenticated requests use the App API. With `web_fallback_enabled=true` and no refresh token, supported read operations use the Web API. Anonymous illustration and ranking lists enrich every record through its detail endpoint before returning; any enrichment error fails the complete list call.

## Writes

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`, optional `restrict`, `tags` | `{success, action, illust_id}` |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `follow_user` | `user_id`, optional `restrict` | `{success, action, user_id}` |
| `unfollow_user` | `user_id` | `{success, action, user_id}` |

Write failures set `success=false` and `isError=true`.
