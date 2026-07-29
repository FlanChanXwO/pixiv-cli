# MCP Tools


## Entity record contract

Every entity-reading tool—including search, detail, recommendations, user lists, feeds, and MyPixiv—returns structured `{records, pagination?}`. Each record has stable string `id`, `type`, and `url`; additional public-SDK fields are preserved without a record/schema/version field. MCP text Content is only a short summary and never duplicates entity JSON. On a classified entity-read error, `isError=true` and `records=[]`. The entity rows below all use this contract; their former per-tool `items`/`illusts`/`user_previews` shapes are not part of v0.8.0 output.

The entity tools are `search_illust`, `search_novel`, `illust_detail`, `illust_related`, `illust_ranking`, `illust_recommended`, `recommended`, `illust_follow`, `novel_follow`, `illust_new`, `novel_new`, `mypixiv_users`, `mypixiv_illusts`, `mypixiv_novels`, `search_user`, `user_detail`, `user_artworks`, `user_novels`, `user_bookmarks`, and `user_following`.

English | [简体中文](../zh-CN/mcp-tools.md) | [日本語](../ja/mcp-tools.md) | [Documentation index](../index.md)

Run `pixiv mcp` to start the stdio server. stdout is reserved for JSON-RPC. Operation logs are written as daily
plain-text files named `YYYY-MM-DD.txt` under `~/.pixiv-cli/logs` (on Windows, `%USERPROFILE%\.pixiv-cli\logs`; default
retention 7 days); the terminal stays free of log traces by default. MCP exposes no HTTP endpoint.

With a refresh token, App API handles the request and returns classified failures. Without a token and with
`web_fallback_enabled=true`, only allowlisted anonymous read tools may use Web API. SDK-based user
detail/list and bookmark/follow write tools return text plus structured output; classified failures set
`isError=true`. Query tools likewise keep a compact text summary and return typed structured content; their failures
set `isError=true`. Legacy session/auth tool failures retain their documented `isError=false` wire behavior, while the
file log records an error-level operation event with `result=error`. Events contain only
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
| `set_refresh_token` | raw App API `refresh_token` | Current-session authentication result; `auth.json` remains unchanged. |
| `download` | exactly one of `src` or `srcs`; each source is a PID, Pixiv artwork/user URL, or allowed CDN URL; optional `pages`, `quality`, `concurrency`, `ugoira_format` (`gif` or `apng`); `delivery` is `local_path` only | `{items, failures, files, text}` with local file metadata; never embeds image bytes. |
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

`download.src` is a single source and `download.srcs` is an ordered source list; supplying both is invalid. A source
can be a local-parsed HTTPS `pixiv.net`/`www.pixiv.net` artwork/user URL, a PID, or a CDN URL accepted by the SDK
resource policy. A user URL uses App OAuth to follow all `illust`, `manga`, and `ugoira` pages; novels are outside
the download set. `concurrency=0` uses
`2 × GOMAXPROCS`; a positive value is used exactly. HTTP cache metadata is retained in `.pixiv-cache`, and only a
validator-matched partial is resumed with `If-Range`. Partial artwork failures remain in `failures` while other works
continue; a partial report has `isError=true`.

Both download tools return valid structured output on validation, SDK, recommendation, download, result-building,
or file-read failure: `delivery` retains the normalized mode (`local_path` when IDs/delivery are invalid), and
`items`, `failures`, and `files` are empty arrays rather than `null`. Validation and all-or-nothing failures retain
the safe business-error text; `download` partial failures set `isError=true`.

## Work and user reads

| Tool | Input | Structured output |
| --- | --- | --- |
| `search_illust` | `word`, `search_target`, `sort`, `duration`, `start_date`, `end_date`, `page`, `limit`, `rating`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, `tool`, `bookmark_min`, `bookmark_max` | `{records, pagination?}`. |
| `search_novel` | `word`, `search_target`, `sort`, `duration`, `page`, `limit`, `rating`, `min_text_length`, `max_text_length`, `original_only` | App-only `{records, pagination?}`. A classified failure has `isError=true`. |
| `search_illust_options` | required `word` | `{tools,text}` for the word; authenticated App only. |
| `illust_detail` | exactly one of `illust_id` or `url` | `{records}`; the complete work detail includes raw HTML `caption` when Pixiv provides it. |
| `illust_related` | `illust_id`, `page`, `limit` | `{records, pagination?}`. |
| `illust_ranking` | `mode`, `date`, `page`, `limit` | `{records, pagination?}`. |
| `illust_recommended` | `page`, `limit` | `{records, pagination?}` through the public SDK path. |
| `recommended` | required `kind` (`all`, `illust`, `manga`, `novel`, `user`), optional `page`, `limit` | `{records, pagination?}`; `all` reads four authenticated streams in order. |
| `trending_tags_illust` | none | `{tags, text}`. |
| `illust_follow` | `restrict`, `page`, `limit` | `{records, pagination?}`; authentication required. |
| `search_user` | `word`, `page`, `limit` | `{records, pagination?}`. The source is `app_search` for official authenticated App search or `related_illust_authors` for anonymous fallback; the latter is not a username search. |
| `user_detail` | required `user_id` | `{records}` with user, profile, profile-publicity, and workspace fields; authenticated App only. |
| `user_artworks` | optional `user_id`, `type`, `page`, `limit` | `{records, pagination?}`; omitted UID uses the authenticated user. |
| `user_bookmarks` | optional `user_id`, `restrict`, `tag`, `page`, `limit` | `{records, pagination?}`. |
| `user_following` | optional `user_id`, `restrict`, `page`, `limit` | `{records, pagination?}`. |

`search_illust` filter values are:

- `rating`: `all|sfw|r18|r18g|mature`;
- `content_type`: `all|illust-and-ugoira|illust|manga|ugoira`;
- `ai_mode`: `all|exclude|only`; Pixiv `AIType==2` means AI-generated;
- `aspect_ratio`: `all|landscape|portrait|square`;
- `resolution`: `all|high|medium|low`, with both dimensions respectively `>=3000`, `1000..2999`, or `<=999`;
- `tool`: exact upstream drawing-tool value, without fuzzy matching.
- `search_target`: `partial_match_for_tags|exact_match_for_tags|title_and_caption|keyword`; `keyword` searches tags, titles, and captions and requires App OAuth.
- `duration`: `within_last_day|within_last_week|within_last_month|within_half_year|within_year`; it cannot be combined with date bounds. The two long values are expanded locally into an inclusive Tokyo date range.
- `start_date` / `end_date`: inclusive `YYYY-MM-DD` bounds. Either is allowed, but a supplied start cannot be later than end.
- `bookmark_min` / `bookmark_max`: inclusive non-negative public bookmark-count bounds; the minimum cannot exceed the maximum and both require App OAuth plus an active Pixiv Premium membership. For a saved account, the cached self-profile status is checked before search, so a non-Premium account fails locally without an upstream search request.

With a refresh token, App performs resolution, aspect ratio, tool, content type, and AI exclusion filtering;
rating and AI-only filtering use public SDK normalized App fields. App failures never fall back to Web. Anonymous
Web applies only verified filters; `r18|r18g|mature`, `search_target=keyword`, and bookmark-count bounds fail before
the request with an authentication requirement. Pixiv additionally requires Premium membership for bookmark-count bounds.

For authenticated `search_illust`, tag `search_target` values preserve the verified Pixiv App query grammar in
`word`: with `exact_match_for_tags`, `tagA tagB` requires both complete tags and uppercase `tagA OR tagB` accepts
either one. Literal `AND` is not an operator. `partial_match_for_tags` also accepts the tested uppercase `OR`, but
uses fuzzy tag terms and must not be treated as strict exact-tag AND. `title_and_caption` and `keyword` have no boolean-tag
contract. No escape syntax for a literal uppercase `OR` tag/keyword is verified.
`search_illust_options` is App-only. None of the search tools accepts cookies.

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
