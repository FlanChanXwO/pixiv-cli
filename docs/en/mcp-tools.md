# MCP Tools

English | [简体中文](../zh-CN/mcp-tools.md) | [日本語](../ja/mcp-tools.md) | [Documentation index](../index.md)

Run `pixiv mcp` to start the stdio server. stdout is reserved for JSON-RPC. Operation logs are written as daily
plain-text files named `YYYY-MM-DD.txt` under `~/.pixiv-cli/logs` (on Windows, `%USERPROFILE%\.pixiv-cli\logs`; default
retention 7 days); the terminal stays free of log traces by default. MCP exposes no HTTP endpoint.

With a refresh token, App API is the primary path and failures never fall back to Web automatically. Without a
token and with `web_fallback_enabled=true`, only allowlisted anonymous read tools may use Web API. SDK-based user
detail/list and bookmark/follow write tools return text plus structured output; classified failures set
`isError=true`. Text-form MCP tool failures retain their documented Content, structured output, text, and
`isError=false` wire behavior, while the file log records an error-level operation event with `result=error`. Events contain only
the operation, stable SDK classification, backend/status, and safe IDs; they never include raw errors, inputs,
queries, tokens, cookies, URLs, paths, or response bodies. Unknown public SDK codes are omitted and unknown
backends normalize to `local`. A normal empty result remains a success.

## Pagination

New SDK list tools use:

- `limit`: maximum items; `0` follows upstream until there is no next batch; omitted means one upstream batch.
- `page`: 1-based logical page and requires a positive `limit`.
- output fields `pagination.page`, `limit`, `returned`, `has_more`, and optional `next_page`.

SDK cursors never appear in MCP input or output. List tools use the logical `page`/`limit` pair.

## Configuration, authentication, and downloads

| Tool | Input | Structured output |
| --- | --- | --- |
| `set_download_path` | `path` | Text status. |
| `refresh_token` | none | Current authenticated-account summary. |
| `set_refresh_token` | raw App API `refresh_token` | Current-session authentication result; does not write `auth.json`; rejects cookies. |
| `download` | `illust_id` or `illust_ids`, optional `pages`/`quality`; `delivery` is `local_path` only | Local file path/file_uri/mime_type/page/size; never embeds image bytes. |
| `download_random_from_recommendation` | optional `count` (omitted/`null` defaults to 5; explicit value must be 1..20), optional `pages`/`quality`; `delivery` is `local_path` only | Text plus structured local file metadata; never embeds image bytes. |

`refresh_token` does not misreport SDK/config/proxy initialization failures as a missing token. Context cancellation
and deadlines retain explicit messages; public `*pixiv.Error` values retain safe code/operation/backend fields;
unknown initialization/execution failures use a redacted diagnostic. Only a real `unauthorized` refresh keeps the
missing-token hint. The tool returns `isError=false`; file-log events expose real failures safely.

`download_random_from_recommendation.count` limits works, not files expanded from each work. Explicit 0, negative,
or values above 20 fail validation instead of being clamped. If fewer recommendations exist, the tool downloads the
available works. Download tools return local file metadata only and never embed image bytes.

When the MCP server is started with an HTTP(S) proxy, its media-resource downloads deliberately use HTTP/1.1. App
API, OAuth, and Web metadata requests retain normal protocol negotiation; this avoids proxy-specific HTTP/2 stream
resets without changing authentication or selected download quality.

Both download tools return valid structured output on validation, SDK, recommendation, download, result-building,
or file-read failure: `delivery` retains the normalized mode (`local_path` when IDs/delivery are invalid), and
`items`/`files` are empty arrays rather than `null`. These failures return `isError=false` and the original safe
business error text.

## Work and user reads

| Tool | Input | Structured output |
| --- | --- | --- |
| `search_illust` | `word`, `search_target`, `sort`, `duration`, `page`, `limit`, `rating`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, `tool` | `{text}`; works remain in text. |
| `search_novel` | `word`, `search_target`, `sort`, `duration`, `page`, `limit`, `rating`, `min_text_length`, `max_text_length`, `original_only` | App-only `{novels, pagination, text}`. A classified failure has `isError=true`. |
| `search_illust_options` | required `word` | `{tools,text}` for the word; authenticated App only. |
| `illust_detail` | `illust_id` | Work detail, including raw HTML `caption` when Pixiv provides it. |
| `illust_related` | `illust_id`, `page`, `limit` | Related works. |
| `illust_ranking` | `mode`, `date`, `page`, `limit` | Ranked works. |
| `illust_recommended` | `page`, `limit` | Illustration recommendation text through the public SDK path. |
| `recommended` | required `kind` (`all`, `illust`, `manga`, `novel`, `user`), optional `page`, `limit` | `{kind, illusts, manga, novels, user_previews, pagination}`; `all` reads four authenticated streams in order. |
| `trending_tags_illust` | none | Trending tags. |
| `illust_follow` | `restrict`, `page`, `limit` | Followed works; authentication required. |
| `search_user` | `word`, `page`, `limit` | `{source, user_previews, pagination, text}`. `source` is `app_search` for official authenticated App search or `related_illust_authors` for anonymous fallback; the latter is not a username search. |
| `user_detail` | required `user_id` | Stable `{user, profile, profile_publicity, workspace}`; authenticated App only. |
| `user_artworks` | optional `user_id`, `type`, `page`, `limit` | `{user_id, items, pagination}`; omitted UID uses the authenticated user. |
| `user_bookmarks` | optional `user_id`, `restrict`, `tag`, `page`, `limit` | `{user_id, items, pagination}`. |
| `user_following` | optional `user_id`, `restrict`, `page`, `limit` | `{user_id, items, pagination}`. |

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
`search_illust_options` is App-only. None of the search tools accepts cookies or bookmark-count filters.

`search_novel.rating` uses `all|sfw|r18|r18g|mature`. `min_text_length` and `max_text_length` are non-negative
character bounds where `0` disables the corresponding bound; a non-zero maximum below the minimum fails validation.
`original_only` keeps only novels marked original. Pixiv has no verified App wire parameters for these three
conditions, so the public SDK validates them against every result's `x_restrict`, `text_length`, and `is_original`
fields. Missing fields are an upstream error, not a silent non-match. `search_novel` itself is App-only.

`search_user` returns text alongside structured `source`, `user_previews`, and `pagination`. Its fixed text explicitly
says when anonymous fallback returned related illustration authors rather
than official username-search results.

Fixed MCP status, error, list-heading, field-label, and ranking text is English. Artwork metadata returned by
Pixiv and tool arguments retain their original text. Artwork text preserves every tag in upstream order without a
five-tag truncation. Known ranking modes use stable English display titles; a future successful mode displays the
raw mode followed by `ranking`.

`illust_ranking.mode` accepts `day`, `day_male`, `day_female`, `week`, `week_original`, `week_rookie`, `month`,
`day_manga`, `week_manga`, `month_manga`, `week_rookie_manga`, `day_r18`, `day_male_r18`, `day_female_r18`,
`week_r18`, and `week_r18g`. The last nine require App authentication and return a classified authentication error
without substituting an anonymous daily ranking. The stable labels are `Daily ranking`, `Daily ranking (male)`,
`Daily ranking (female)`, `Weekly ranking`, `Weekly original ranking`, `Weekly rookie ranking`, `Monthly ranking`,
`Daily manga ranking`, `Weekly manga ranking`, `Monthly manga ranking`, `Weekly rookie manga ranking`,
`Daily R-18 ranking`, `Daily male R-18 ranking`, `Daily female R-18 ranking`, `Weekly R-18 ranking`, and
`Weekly R-18G ranking`, in the same order as the modes above.

## Writes

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`, optional `restrict`, `tags` | `{success, action, illust_id}`. |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}`. |
| `follow_user` | `user_id`, optional `restrict` | `{success, action, user_id}`. |
| `unfollow_user` | `user_id` | `{success, action, user_id}`. |

All writes use the SDK and require authentication. Failures return `success=false` and MCP `isError=true`.
