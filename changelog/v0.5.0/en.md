# v0.5.0 — 2026-07-22

## Added

- CLI and MCP operation summaries are written as daily JSONL files under the user state directory at `pixiv/logs`. Logs retain seven days by default, clean up only recognized historical files, stay silent in the terminal, and contain only redacted operation summaries. Exceptional non-authentication upstream failures can point CLI users to the log directory; login failures and expired tokens do not.
- MCP `download` and `download_random_from_recommendation` now accept `pages` and `quality`, sharing the CLI download options.
- Downloads support 1-based `--pages` (`1,3-5`) and `--quality original|regular|small|thumb|mini`; the default remains all original pages. Ugoira rejects derived qualities and page selection. The public SDK exposes `ParsePageSpec`, `DownloadQuality`, and `DownloadOptions`.
- Artwork models and all CLI/MCP output forms now include the stable artwork `url` (`https://www.pixiv.net/artworks/${id}`).
- Public `UgoiraMetadata` adds paired, non-empty `download_url` and `download_quality` (`medium|original`). `zip_urls.original` is emitted only when an original ZIP was actually obtained; the downloader, CLI, and MCP use the verified resource.
- Illustration ranking supports all 16 App API modes, including four manga and five R18 modes. CLI `--mode`, MCP stable labels, and public SDK constants support them; the newly added modes explicitly require authentication.

## Changed

- CLI prompts, OAuth completion pages, log-directory hints, and fixed help examples are English. Artwork metadata and user input are unchanged.
- **Breaking:** fixed MCP status, error, and display text is English; structured data, Pixiv metadata, and user input are unchanged.
- **Breaking:** removed `pixiv search --search-target`, `--target`, `--duration`, and `--tool` without aliases. Use `--search-by tag-partial|tag-exact|title-caption`, `--period day|week|month`, and `--draw-tool`; the public `--limit` default no longer exposes internal sentinel `-1`.
- **Breaking:** `--download-path` and `--filename-template` are accepted only by `pixiv download`; other commands now reject these previously ignored no-op flags.
- **Breaking:** diagnostics are written to the user state directory instead of stderr; MCP stdout remains JSON-RPC only.
- **Breaking:** MCP download returns only local `path`, `file_uri`, `mime_type`, page number, and size. `delivery=image_content` and `get_thumbnail_base64` were removed; agents should send the local file through host attachment support or share the artwork URL.
- **Breaking:** removed MCP wire fields `search_r18`, `user_id_to_check`, `max_bookmark_id`, `offset`, and `include_thumbnail`; lists and search use `user_id`, `rating`, and `page`/`limit`.
- **Breaking:** removed CLI compatibility inputs `--ai-type`, `--r18`, `--profile`, `--offset`, and `search --type comics`. Use `--ai-mode`, `--rating r18`, `--uid`, `--page`/`--limit`, and `--type manga`.

## Fixed

- Closed daily JSONL log files when a CLI command or MCP session ends, allowing Windows state-directory cleanup immediately after return.
- Authenticated illustration detail, pages, and ugoira metadata now use only successful App API data, preventing R18 403/404 failures caused by anonymous Web enrichment. Multi-page works use `meta_pages`; single pages are derived canonically; missing or mismatched App data returns an explicit upstream-response error rather than a partial result.
- Idempotent App API JSON reads recover once from an initial HTTP 429 with a valid `Retry-After`, waiting through the caller context. Invalid headers, a second 429, writes, and resource downloads retain the real error and are never replayed; retry logs omit URLs, headers, and credentials.
- The macOS `pixiv://` helper now opens a local bridge page and shows a centered final success/failure page after OAuth completes. Callback data is held only briefly in a URL fragment and removed before submission, failures do not reveal sensitive reasons, and the CLI success message starts on a new line.
- Search now fetches through consecutive empty upstream batches created by local filters, fills `--limit N`/`limit`, traverses for `0`, and applies `--page`/`page` after filtering.
- App artwork AI detection prioritizes `illust_ai_type` while accepting legacy `ai_type`; local AI classification remains `AIType == 2`.
