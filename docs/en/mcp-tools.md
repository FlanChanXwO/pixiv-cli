# MCP Tools

English | [简体中文](../zh-CN/mcp-tools.md) | [Documentation index](../index.md)

Run `pixiv mcp` to start the stdio server. stdout is reserved for JSON-RPC and logs go to stderr. MCP exposes no
HTTP endpoint.

With a refresh token, App API is the primary path and failures never fall back to Web automatically. Without a
token and with `web_fallback_enabled=true`, only allowlisted anonymous read tools may use Web API. SDK-based user
detail/list and bookmark/follow write tools return text plus structured output; classified failures set
`isError=true`. Legacy tool failures retain their existing Content, structured output, text, and `isError=false`
wire behavior, while stderr records an error-level operation event with `result=error`. Events contain only the
operation, stable SDK classification, backend/status, and safe IDs; they never include raw errors, inputs, queries,
tokens, cookies, URLs, paths, or response bodies. Unknown public SDK codes are omitted and unknown backends
normalize to `local`. A normal empty result remains a success.

## Pagination

New SDK list tools use:

- `limit`: maximum items; `0` follows upstream until there is no next batch; omitted means one upstream batch.
- `page`: 1-based logical page and requires a positive `limit`.
- output fields `pagination.page`, `limit`, `returned`, `has_more`, and optional `next_page`.

SDK cursors never appear in MCP input or output. `user_bookmarks.max_bookmark_id` is a deprecated legacy
continuation and cannot be combined with `page` or `limit`. `user_following.offset` is a deprecated logical offset;
it conflicts only with `page` and may still be used with `limit`.

## Configuration, authentication, and downloads

| Tool | Input | Structured output |
| --- | --- | --- |
| `set_download_path` | `path` | Text status. |
| `refresh_token` | none | Current authenticated-account summary. |
| `set_refresh_token` | raw App API `refresh_token` | Current-session authentication result; does not write `auth.json`; rejects cookies. |
| `download` | `illust_id` or `illust_ids`, optional `delivery` | Files, URI, MIME, and size; `image_content` also attaches ImageContent. |
| `download_random_from_recommendation` | optional `count` (omitted/`null` defaults to 5; explicit value must be 1..20), optional `delivery` | Text plus structured file metadata; never attaches ImageContent. |

`refresh_token` does not misreport SDK/config/proxy initialization failures as a missing token. Context cancellation
and deadlines retain explicit messages; public `*pixiv.Error` values retain safe code/operation/backend fields;
unknown initialization/execution failures use a redacted diagnostic. Only a real `unauthorized` refresh keeps the
missing-token hint. This legacy tool remains `isError=false`; stderr events expose real failures safely.

`download_random_from_recommendation.count` limits works, not files expanded from each work. Explicit 0, negative,
or values above 20 fail validation instead of being clamped. If fewer recommendations exist, the tool downloads the
available works. It never attaches ImageContent, including with `delivery=image_content`.

Both download tools return valid structured output on validation, SDK, recommendation, download, result-building,
or file-read failure: `delivery` retains the normalized mode (`local_path` when IDs/delivery are invalid), and
`items`/`files` are empty arrays rather than `null`. Legacy failures keep `isError=false` and the original safe
business error text.

## Work and user reads

| Tool | Input | Structured output |
| --- | --- | --- |
| `search_illust` | `word`, `search_target`, `sort`, `duration`, `offset`, `rating`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, `tool`, compatibility `search_r18`, `include_thumbnail` | Legacy `{text}`; works remain in text. |
| `search_illust_options` | required `word` | `{tools,text}` for the word; authenticated App only. |
| `illust_detail` | `illust_id` | Work detail. |
| `illust_related` | `illust_id`, `offset`, `include_thumbnail` | Related works. |
| `illust_ranking` | `mode`, `date`, `offset`, `include_thumbnail` | Ranked works. |
| `illust_recommended` | `offset`, `include_thumbnail` | Legacy illustration recommendation text through the public SDK path. |
| `recommended` | required `kind` (`all`, `illust`, `manga`, `novel`, `user`), optional `page`, `limit` | `{kind, illusts, manga, novels, user_previews, pagination}`; `all` reads four authenticated streams in order. |
| `trending_tags_illust` | none | Trending tags. |
| `illust_follow` | `restrict`, `offset`, `include_thumbnail` | Followed works; authentication required. |
| `search_user` | `word`, `offset` | Users; anonymous fallback deduplicates related-work authors and is not official username search. |
| `get_thumbnail_base64` | `illust_id` | `data:image/jpeg;base64,...`. |
| `user_detail` | required `user_id` | Stable `{user, profile, profile_publicity, workspace}`; authenticated App only. |
| `user_artworks` | optional `user_id`, `type`, `page`, `limit` | `{user_id, items, pagination}`; omitted UID uses the authenticated user. |
| `user_bookmarks` | optional `user_id`, legacy `user_id_to_check`, `restrict`, `tag`, `page`, `limit`, deprecated `max_bookmark_id` | `{user_id, items, pagination}`. |
| `user_following` | optional `user_id`, legacy `user_id_to_check`, `restrict`, `page`, `limit`, deprecated `offset` | `{user_id, items, pagination}`. |

`search_illust` filter values are:

- `rating`: `all|sfw|r18|r18g|mature`;
- `content_type`: `all|illust-and-ugoira|illust|manga|ugoira`;
- `ai_mode`: `all|exclude|only`; Pixiv `AIType==2` means AI-generated;
- `aspect_ratio`: `all|landscape|portrait|square`;
- `resolution`: `all|high|medium|low`, with both dimensions respectively `>=3000`, `1000..2999`, or `<=999`;
- `tool`: exact upstream drawing-tool value, without fuzzy matching.

With a refresh token, App performs resolution, aspect ratio, tool, content type, and AI exclusion filtering;
rating and AI-only filtering use public SDK normalized App fields. App failures never fall back to Web. Anonymous
Web applies only verified filters; `r18|r18g|mature` fails before the request with an authentication requirement.
`search_r18=true` maps to `r18` only when `rating` is absent; supplying both conflicts. `search_illust_options` is
App-only. Neither search tool accepts cookies or bookmark-count filters.

Artwork text preserves every tag in upstream order without a five-tag truncation. Known ranking modes use stable
Chinese display titles for legacy compatibility; a future successful mode displays the raw mode followed by
“排行榜”.

## Writes

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`, optional `restrict`, `tags` | `{success, action, illust_id}`. |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}`. |
| `follow_user` | `user_id`, optional `restrict` | `{success, action, user_id}`. |
| `unfollow_user` | `user_id` | `{success, action, user_id}`. |

All writes use the SDK and require authentication. Failures return `success=false` and MCP `isError=true`.
