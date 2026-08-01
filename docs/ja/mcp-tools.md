# MCP ツール

[English](../en/mcp-tools.md) | [简体中文](../zh-CN/mcp-tools.md) | 日本語 | [Documentation index](../index.md)

stdio server は `pixiv mcp` で起動します。起動前に CLI で account と local download setting を設定してください。MCP は runtime configuration に従って選択済み local account または `PIXIV_REFRESH_TOKEN` を使用します。

すべての entity-read tool は structured `{records, pagination?}` を返します。各 Record は stable な `id`、`type`、`url` と該当する public SDK field を持ち、text Content は同じ操作の短い summary です。

## Error と pagination

MCP schema に合わない input は JSON-RPC schema error です。tool 実行開始後の失敗は、documented structured result を保持し `isError=true` を設定します。download は report shape を、entity read は `records: []` を保持します。通常の empty result は成功です。

List tool は `limit` と `page` を受け取ります。

- `limit` 省略時は one upstream batch、`limit: 0` は upstream end まで読みます。
- `page` は 1-based logical page で、positive `limit` が必要です。
- filter は logical pagination より前に適用されます。`type + id` で重複を除き、要求 page が埋まるか upstream が終わるまで読み続けます。

SDK cursor は server 内部に保持されます。list result は `pagination.page`、`limit`、`returned`、`has_more`、必要に応じて `next_page` を出力します。

## Entity filter

Entity list は entity 名に対応する optional filter を使えます。

| Filter | Field |
| --- | --- |
| `illust_filter` | `id`（positive）、`type`（`illust`、`manga`、`ugoira`）、`tags`（すべて exact）、`min_views`（non-negative）、`min_pages`（non-negative） |
| `novel_filter` | `id`（positive）、`tags`（すべて exact）、`min_views`（non-negative） |
| `user_filter` | `id`（positive） |

Illustration list は `illust_filter`、novel list は `novel_filter`、user list は `user_filter` を受け取ります。mixed recommendation には record kind ごとの filter を渡せます。すべての illustration list tool と `download` は top-level `filter` string も受け取り、公開 illustration field に対する safe local expression を `illust_filter` と AND で組み合わせます。比較、`and`/`or`/`not`、`in`/`not in`、array、`any`/`all` のみを使えます。例: `bookmarkCount >= 5000 and xRestrict == 0`、`any(tags, # in ["miku"])`。mixed result で `filter` を使うと novel と user record は除外されます。

## Download

| Tool | Input | Structured output |
| --- | --- | --- |
| `download` | `src` または ordered `srcs` のどちらか一方。PID、supported Pixiv artwork/user/public-bookmarks/illustration-series URL、または allowed CDN URL。optional `pages`（`3-` を含む）、`quality`、`concurrency`、`ugoira_mode`（`gif`、`apng`、`zip`、`frames`）、`filter`、`archive`、`directory_template`、`write_metadata`、`retries`、`retry_delay`、`delivery: "local_path"`。 | local-file metadata を持つ `{items, failures, files, text}`。 |
| `download_random_from_recommendation` | optional `count`（omit または `null` は 5、explicit は 1..20）、optional `pages`/`quality`/`ugoira_mode`、`delivery: "local_path"`。 | 同じ error semantics の local-file report。 |

User/public-bookmarks/illustration-series URL は authentication 済みの illustration、manga、ugoira を source order で展開し、重複 artwork ID は一度だけ取得します。`filter` は artwork detail 取得後、file write 前に適用され、artwork metadata を持たない CDN URL は `filter` を拒否します。`archive` は SQLite file で、選択された output と requested metadata sidecar がすべて成功した artwork のみを記録します。directory/file name template は `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}` を使えます。`concurrency: 0` は `2 × GOMAXPROCS` を使います。resource request は default で 3 回 retry（1/2/4 seconds、有効な `Retry-After` を優先）し、resource cache は validator が一致する partial file を安全に resume します。失敗 item は `failures` に残り、独立した item は継続し、いずれかの failure で `isError=true` になります。

## Read

| Tool | Input |
| --- | --- |
| `search_illust` | `word`、search filter、`page`、`limit`、optional `filter` と `illust_filter` |
| `search_novel` | `word`、novel-search filter、`page`、`limit`、optional `novel_filter` |
| `illust_detail` | `illust_id` または supported `url` の一方 |
| `illust_related`、`illust_ranking`、`illust_recommended` | operation input、`page`、`limit`、optional `filter` と `illust_filter` |
| `recommended` | required `kind`（`all`、`illust`、`manga`、`novel`、`user`）、`page`、`limit`、optional `filter`、applicable entity filter |
| `trending_tags_illust` | input なし。`{tags, text}` を返します。 |
| `timeline_illust_following` | `restrict`、`page`、`limit`、optional `filter` と `illust_filter` |
| `timeline_novel_following` | `restrict`、`page`、`limit`、optional `novel_filter` |
| `timeline_illust_latest` | required `content_type`（`illust` または `manga`）、`page`、`limit`、optional `filter` と `illust_filter` |
| `timeline_novel_latest` | `page`、`limit`、optional `novel_filter` |
| `mypixiv_users` | `page`、`limit`、optional `user_filter` |
| `mypixiv_illusts` / `mypixiv_novels` | `page`、`limit`、illustration の optional top-level `filter`、対応する illustration または novel filter |
| `search_user` | `word`、`page`、`limit`、optional `user_filter` |
| `user_detail` | required `user_id` |
| `user_artworks`、`user_novels`、`user_bookmarks`、`user_following` | optional `user_id`、operation-specific input、`page`、`limit`、illustration list の optional top-level `filter`、対応する entity filter |

`search_illust.tool` は [CLI reference](cli-reference.md#drawing-tool-catalog) の versioned drawing-tool catalog を使います。exact value が必要で、unique な 1-edit spelling correction は validation error に示され、ambiguous prefix は error になります。

`illust_ranking.mode` は `day`、`day_male`、`day_female`、`week`、`week_original`、`week_rookie`、`month`、`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、`day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g` を受け取ります。後半の九つは authentication が必要です。

Authentication 済み request は App API を使います。refresh token がなく `web_fallback_enabled=true` の場合、supported read operation は Web API を使います。anonymous illustration/ranking list は各 record を detail endpoint で補完してから返し、補完 failure は list call 全体の failure になります。

## Write

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`、optional `restrict`、`tags` | `{success, action, illust_id}` |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `follow_user` | `user_id`、optional `restrict` | `{success, action, user_id}` |
| `unfollow_user` | `user_id` | `{success, action, user_id}` |

Write failure は `success=false` と `isError=true` を返します。
