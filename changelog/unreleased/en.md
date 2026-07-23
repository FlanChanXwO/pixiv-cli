# Unreleased

## Added

- Authenticated App API novel search is now available through `pixiv novel search WORD`, the public `SearchNovel` SDK API, and MCP `search_novel`. It supports keyword matching, date sort, time range, rating, text length, and original-only filters; results include a stable artwork URL, rating, text length, and original flag.
- Added `pixiv user search WORD` and expanded MCP `search_user` to return `{source,user_previews,pagination,text}`. Results identify either the official App user search (`app_search`) or the anonymous related-illustration-author fallback (`related_illust_authors`) so the fallback is never presented as a username search.

## Changed

- Persistent local application data now lives directly in `~/.pixiv-cli` on macOS/Linux and `%USERPROFILE%\.pixiv-cli` on Windows, including authentication, configuration, callback-bridge state, logs, the Release-check cache, and the macOS callback helper. Earlier storage paths are not read or migrated.
- Artwork detail now includes `caption`. The SDK, CLI JSON, and MCP preserve Pixiv's original HTML; the normal CLI detail view safely renders plain text. Lists do not add captions.
- Release tags must now pass the complete authenticated E2E suite in the protected `pixiv-e2e` Environment. Pull-request and `main` CI remain offline and secret-free; a real-regression failure blocks production build and publication.
- Operation diagnostics in the CLI, MCP, SDK, downloader, and App API now use safe structured events that never contain tokens, URLs, raw headers, request inputs, or response bodies.

## Fixed

- Fixed browser OAuth callbacks on desktop Linux and Windows: `pixiv://` is now registered only for the active login and the prior user association is restored afterward. The SSH `--no-open --addr` fallback page now continues a validated Pixiv relay in the browser that submitted it, so a local `ssh -L` tunnel can return the final callback to a GUI-less server without a second pixiv installation.
- Fixed possible HTTP/2 resource-stream interruptions while downloading static illustrations or ugoira through an explicitly configured HTTP(S) proxy. Resource transfers now negotiate HTTP/1.1 independently; App API, OAuth, and Web metadata requests retain their existing protocol negotiation.
- File logs now use plain-text `YYYY-MM-DD.txt` names without a `pixiv-` prefix; JSONL output and the unused `log_format` / `PIXIV_LOG_FORMAT` switches were removed. OAuth login, callback, success, and failure pages now keep the browser title `pixiv-cli`.
- OAuth operation diagnostics now retain the safe `transport_kind` classification, including typed network timeouts, so an upstream failure without an HTTP status can be diagnosed without logging URLs, credentials, headers, or response bodies.
