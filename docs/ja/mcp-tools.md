# MCP ツール

[English](../en/mcp-tools.md) | [简体中文](../zh-CN/mcp-tools.md) | 日本語 | [ドキュメント索引](../index.md)

`pixiv mcp` で stdio server を起動します。stdout は JSON-RPC 専用です。操作ログは
`~/.pixiv-cli/logs`（Windows では `%USERPROFILE%\.pixiv-cli\logs`）配下の、日付ごとの plain-text file
`YYYY-MM-DD.txt` に記録されます（既定の保持期間は 7 日）。既定では端末へログを出力しません。MCP は HTTP endpoint を公開しません。

refresh token がある場合、App API が主経路となり、失敗時に Web へ自動 fallback しません。token がなく、かつ
`web_fallback_enabled=true` の場合に限り、allowlist にある匿名 read tool が Web API を利用できます。SDK 経由の
ユーザー詳細・ユーザー一覧・bookmark/follow write tool は text と structured output の両方を返し、分類可能な失敗では
`isError=true` になります。text 形式の MCP tool は失敗時も既定の Content、structured output、text、`isError=false`
という wire behavior を維持しますが、file log には `result=error` の error-level operation event を記録します。event に含むのは
operation、安定した SDK 分類、backend/status、安全な ID だけです。raw error、入力、query、token、cookie、URL、path、response body
は記録しません。未知の public SDK code は記録せず、未知の backend は `local` に正規化します。通常の空結果は成功です。

## ページネーション

新しい SDK list tool は次を使用します。

- `limit`：最大件数。`0` は上流に次の batch がなくなるまで追跡し、省略時は 1 upstream batch を取得します。
- `page`：1 始まりの論理 page。正の `limit` が必要です。
- output field：`pagination.page`、`limit`、`returned`、`has_more`、および任意の `next_page`。

SDK cursor は MCP の入力・出力に現れません。list tool は論理的な `page`/`limit` の組を使用します。

## 設定、認証、ダウンロード

| Tool | Input | Structured output |
| --- | --- | --- |
| `set_download_path` | `path` | Text status。 |
| `refresh_token` | なし | 現在認証済みの account summary。 |
| `set_refresh_token` | raw App API `refresh_token` | 現在 session の認証結果。`auth.json` には書き込まず、cookie を拒否します。 |
| `download` | `illust_id` または `illust_ids`、任意の `pages`/`quality`。`delivery` は `local_path` のみ | local file の path/file_uri/mime_type/page/size。image bytes は埋め込みません。 |
| `download_random_from_recommendation` | 任意の `count`（省略/`null` は既定で 5、明示値は 1..20）、任意の `pages`/`quality`。`delivery` は `local_path` のみ | text と structured な local file metadata。image bytes は埋め込みません。 |

`refresh_token` は SDK/config/proxy の初期化失敗を「token がない」と誤認しません。context の cancellation/deadline は明示的な
message を維持し、public `*pixiv.Error` は安全な code/operation/backend field を維持します。未知の初期化・実行失敗は
脱敏した診断になります。実際の refresh で `unauthorized` の場合だけ missing-token hint を返します。この tool の wire result は
`isError=false` ですが、file-log event は実際の失敗を安全に記録します。

`download_random_from_recommendation.count` は、各 work から展開される file 数ではなく work 数を制限します。明示的な
0、負数、20 より大きい値は clamp せず validation error になります。recommendation が少ない場合は利用可能な work を
download します。download tool は local file metadata だけを返し、image bytes を埋め込みません。

MCP server を HTTP(S) proxy とともに起動した場合、その media-resource download は意図的に HTTP/1.1 を使用します。
App API、OAuth、Web metadata request は通常の protocol negotiation を維持します。これは proxy 固有の HTTP/2 stream reset を
回避するためであり、認証や選択する download quality は変更しません。

どちらの download tool も validation、SDK、recommendation、download、result building、file read で失敗しても有効な
structured output を返します。`delivery` は正規化済みの `local_path` を維持し（ID または delivery が無効でも同じ）、
`items`/`files` は `null` ではなく空 array です。これらの失敗は `isError=false` と元の安全な business error text を返します。

## 作品とユーザーの読み取り

| Tool | Input | Structured output |
| --- | --- | --- |
| `search_illust` | `word`、`search_target`、`sort`、`duration`、`page`、`limit`、`rating`、`content_type`、`ai_mode`、`aspect_ratio`、`resolution`、`tool` | `{text}`。works は text に残ります。 |
| `search_novel` | `word`、`search_target`、`sort`、`duration`、`page`、`limit`、`rating`、`min_text_length`、`max_text_length`、`original_only` | App 専用の `{novels, pagination, text}`。分類可能な失敗は `isError=true`。 |
| `search_illust_options` | 必須 `word` | 検索語の `{tools,text}`。認証済み App 専用。 |
| `illust_detail` | `illust_id` | Pixiv が提供する場合は raw HTML `caption` を含む作品詳細。 |
| `illust_related` | `illust_id`、`page`、`limit` | 関連作品。 |
| `illust_ranking` | `mode`、`date`、`page`、`limit` | ranking works。 |
| `illust_recommended` | `page`、`limit` | public SDK path による illustration recommendation text。 |
| `recommended` | 必須 `kind`（`all`、`illust`、`manga`、`novel`、`user`）、任意の `page`、`limit` | `{kind, illusts, manga, novels, user_previews, pagination}`。`all` は 4 つの認証済み stream を順に読みます。 |
| `trending_tags_illust` | なし | trending tags。 |
| `illust_follow` | `restrict`、`page`、`limit` | follow 中の新作。認証が必要。 |
| `search_user` | `word`、`page`、`limit` | `{source, user_previews, pagination, text}`。認証済み App では `app_search`、匿名 fallback では `related_illust_authors`。後者は username search ではありません。 |
| `user_detail` | 必須 `user_id` | 安定した `{user, profile, profile_publicity, workspace}`。認証済み App 専用。 |
| `user_artworks` | 任意の `user_id`、`type`、`page`、`limit` | `{user_id, items, pagination}`。UID の省略時は認証済み user を使います。 |
| `user_bookmarks` | 任意の `user_id`、`restrict`、`tag`、`page`、`limit` | `{user_id, items, pagination}`。 |
| `user_following` | 任意の `user_id`、`restrict`、`page`、`limit` | `{user_id, items, pagination}`。 |

`search_illust` の filter value は次のとおりです。

- `rating`：`all|sfw|r18|r18g|mature`。
- `content_type`：`all|illust-and-ugoira|illust|manga|ugoira`。
- `ai_mode`：`all|exclude|only`。Pixiv の `AIType==2` は AI-generated を意味します。
- `aspect_ratio`：`all|landscape|portrait|square`。
- `resolution`：`all|high|medium|low`。両 dimension がそれぞれ `>=3000`、`1000..2999`、`<=999` です。
- `tool`：fuzzy matching をしない、上流の drawing-tool value そのもの。

refresh token がある場合、resolution、aspect ratio、tool、content type、AI exclusion は App が実施します。rating と
AI-only の filter は public SDK が正規化済み App field で実施します。App の失敗は Web に fallback しません。匿名 Web は
検証済みの filter のみを適用し、`r18|r18g|mature` は request 前に authentication requirement で失敗します。
`search_illust_options` は App 専用です。いずれの search tool も cookie または bookmark-count filter を受け取りません。

`search_novel.rating` は `all|sfw|r18|r18g|mature` を使用します。`min_text_length` と `max_text_length` は非負の
character bound で、`0` は対応する bound を無効にします。非ゼロの maximum が minimum 未満なら validation error です。
`original_only` は original とされた novel のみを残します。Pixiv にはこれら 3 条件の検証済み App wire parameter がないため、
public SDK は各 result の `x_restrict`、`text_length`、`is_original` で検証します。field の欠落は upstream error であり、
暗黙の non-match ではありません。`search_novel` 自体が App 専用です。

`search_user` は text と structured な `source`、`user_previews`、`pagination` を返します。匿名 fallback 時の固定 text は、
公式 username-search result ではなく related illustration author を返したことを明示します。

固定の MCP status、error、list heading、field label、ranking text は英語です。Pixiv が返す artwork metadata と
tool argument は元の text を保持します。artwork text は upstream order の全 tag を保持し、5 tag で切り詰めません。
既知の ranking mode は安定した英語 title を使い、将来の成功した mode は raw mode の後に `ranking` を表示します。

`illust_ranking.mode` は `day`、`day_male`、`day_female`、`week`、`week_original`、`week_rookie`、`month`、
`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、`day_male_r18`、`day_female_r18`、
`week_r18`、`week_r18g` を受け取ります。最後の 9 mode は App 認証が必要で、匿名 daily ranking に置き換えることなく
分類済み authentication error を返します。安定 label は順に `Daily ranking`、`Daily ranking (male)`、
`Daily ranking (female)`、`Weekly ranking`、`Weekly original ranking`、`Weekly rookie ranking`、`Monthly ranking`、
`Daily manga ranking`、`Weekly manga ranking`、`Monthly manga ranking`、`Weekly rookie manga ranking`、
`Daily R-18 ranking`、`Daily male R-18 ranking`、`Daily female R-18 ranking`、`Weekly R-18 ranking`、
`Weekly R-18G ranking` です。

## 書き込み操作

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`、任意の `restrict`、`tags` | `{success, action, illust_id}`。 |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}`。 |
| `follow_user` | `user_id`、任意の `restrict` | `{success, action, user_id}`。 |
| `unfollow_user` | `user_id` | `{success, action, user_id}`。 |

すべての write は SDK を使い、認証が必要です。失敗時は `success=false` および MCP `isError=true` を返します。
