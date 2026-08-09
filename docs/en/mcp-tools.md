# MCP tools

English | [简体中文](../zh-CN/mcp-tools.md) | [Documentation index](../index.md)

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

An illustration list accepts `illust_filter`, a novel list accepts `novel_filter`, and a user list accepts `user_filter`. Mixed recommendations accept the matching filter for each record kind. Every illustration-list tool and `download` also accept a top-level `filter` string: a safe local expression over public illustration fields. It is combined with `illust_filter` using AND. Expressions support comparisons, `and`/`or`/`not`, `in`/`not in`, arrays, and `any`/`all`; for example, `bookmarkCount >= 5000 and xRestrict == 0` or `any(tags, # in ["miku"])`. When a mixed result uses `filter`, novels and users are omitted.

## Downloads

| Tool | Input | Structured output |
| --- | --- | --- |
| `download` | Exactly one of `src` or ordered `srcs`; each source is a PID, supported Pixiv artwork/user/bookmarks/series URL, or allowed CDN URL. Optional `pages` (including `3-`), `quality`, `concurrency`, `ugoira_mode` (`gif`, `apng`, `zip`, `frames`), `filter`, `archive`, `directory_template`, `write_metadata`, `retries`, `retry_delay`, and `delivery: "local_path"`. | `{items, failures, files, text}` with local-file metadata. |
| `download_random_from_recommendation` | Optional `count` (omitted or `null` is 5; explicit values are 1..20), optional `pages`, `quality`, `ugoira_mode`, and `delivery: "local_path"`. | Local-file report using the same error semantics. |

User, public-bookmarks, and illustration-series URLs expand authenticated illustration, manga, and ugoira works in source order; duplicate artwork IDs are fetched once. A filter runs after artwork detail is available and before files are written; a CDN URL has no artwork metadata and therefore rejects `filter`. `archive` is a SQLite file that records an artwork only after every selected output and requested metadata sidecar has succeeded. `directory_template` and filename templates support `{id}`, `{title}`, `{author}`, `{author_id}`, `{date}`, `{tags}`, and `{num}`. `concurrency: 0` uses `2 × GOMAXPROCS`. Resource requests retry retryable failures three times by default (1/2/4 seconds; a valid `Retry-After` takes precedence) and the resource cache safely resumes a validator-matched partial file. A failed item remains in `failures` while independent items continue, and any failure sets `isError=true`.

## Reads

| Tool | Input |
| --- | --- |
| `search_illust` | `word`, search filters, `page`, `limit`, optional `filter` and `illust_filter` |
| `search_novel` | `word`, novel-search filters, `page`, `limit`, optional `novel_filter` |
| `illust_detail` | Exactly one of `illust_id` or supported `url` |
| `illust_related`, `illust_ranking`, `illust_recommended` | Their operation inputs, `page`, `limit`, optional `filter` and `illust_filter` |
| `recommended` | Required `kind` (`all`, `illust`, `manga`, `novel`, `user`), `page`, `limit`, optional `filter`, and applicable entity filters |
| `trending_tags_illust` | No input; returns `{tags, text}` |
| `timeline_illust_following` | `restrict`, `page`, `limit`, optional `filter` and `illust_filter` |
| `timeline_novel_following` | `restrict`, `page`, `limit`, optional `novel_filter` |
| `timeline_illust_latest` | Required `content_type` (`illust` or `manga`), `page`, `limit`, optional `filter` and `illust_filter` |
| `timeline_novel_latest` | `page`, `limit`, optional `novel_filter` |
| `mypixiv_users` | `page`, `limit`, optional `user_filter` |
| `mypixiv_illusts` / `mypixiv_novels` | `page`, `limit`, optional top-level `filter` for illustrations, and the matching illustration or novel filter |
| `search_user` | `word`, `page`, `limit`, optional `user_filter` |
| `user_detail` | Required `user_id` |
| `user_artworks`, `user_novels`, `user_bookmarks`, `user_following` | Optional `user_id`, operation-specific inputs, `page`, `limit`, optional top-level `filter` for illustration lists, and the matching entity filter |

`search_illust.tool` uses the versioned drawing-tool catalog from the [CLI reference](cli-reference.md#drawing-tool-catalog). It requires an exact value; a unique one-edit spelling correction is offered in the validation error, while ambiguous prefixes are rejected.

Every read requires a valid access token against the Pixiv App API. There is no anonymous or Web fallback; the removed `web_fallback_enabled` setting returns `removed_setting` if still present.

## Writes

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`, optional `restrict`, `tags` | `{success, action, illust_id}` |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `follow_user` | `user_id`, optional `restrict` | `{success, action, user_id}` |
| `unfollow_user` | `user_id` | `{success, action, user_id}` |

Write failures set `success=false` and `isError=true`.

## FANBOX MCP server

Start the independent read-only server with `pixiv fanbox mcp`. It uses the
selected local FANBOX account and never exposes session values as tool input or
output. The registered tools are:

| Tool | Purpose |
| --- | --- |
| `fanbox_current_user` | Validate the current FANBOX session and return a safe identity summary. |
| `fanbox_creator`, `fanbox_creators` | Read one creator profile or supporting/following creators. |
| `fanbox_creator_tags` | Read creator tags. |
| `fanbox_creator_posts`, `fanbox_tagged_posts` | Read creator or tagged posts with SDK cursors. |
| `fanbox_post`, `fanbox_home`, `fanbox_supporting` | Read one post or a feed. |
| `fanbox_resolve_url` | Resolve a supported FANBOX page URL into a typed reference. |
| `fanbox_open_resource` | Validate and open a FANBOX media reference, returning status and headers without bytes. |

Pixiv tools and FANBOX tools are registered by separate servers and do not
cross product credentials, proxy settings, or routes. A FANBOX native
`--proxy`/`--no-proxy` override affects only native FANBOX requests; the
optional FlareSolverr service and its upstream proxy remain independent.

## Debug and stdout

`pixiv --debug mcp` and `pixiv --debug fanbox mcp` write typed, safe lifecycle,
network, challenge, solver, download, and failure diagnostics to stderr. The
MCP stdout stream remains pure JSON-RPC, tool schemas and structured failures
are unchanged, and each server uses its own local request number. No raw URL
query, Cookie, token, proxy userinfo, or FlareSolverr clearance is emitted.
