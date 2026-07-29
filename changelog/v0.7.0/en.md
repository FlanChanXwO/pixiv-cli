# v0.7.0 — 2026-07-25

## Added

- Release-binary updates and versioned installer assets now carry a static, tested free GitHub Release-source list. They probe viable routes locally, support path and query-parameter proxy templates, and never fetch a remote mirror list.
- Existing `detail` and `download` inputs now accept strict official Pixiv artwork URLs; `download` also accepts authenticated user profile and artworks URLs to walk every illustration, manga, and ugoira without downloading novels.
- MCP `illust_detail` accepts exactly one of an ID or URL, and `download` accepts URLs. Illustration query tools now return typed structured results alongside compact text summaries.
- Illustration `search` and MCP `search_illust` now support official App `keyword` search (tags, titles, and captions), inclusive explicit date bounds, public bookmark-count bounds, and the `half-year` / `year` quick date ranges. Bookmark-count bounds require both App OAuth and an active Pixiv Premium membership. App-only filters return an authentication requirement when App OAuth is absent.
- Each published GitHub Release now submits the matching tagged `pixiv-cli` product skill to SkillHub. The submission is validated locally first and the workflow requires the returned SkillHub `skillId` and review status before succeeding.

## Changed

- A configured `--proxy`, `https_proxy`, or `HTTPS_PROXY` keeps precedence over public Release sources. The updater preserves canonical GitHub release identity and Ed25519/SHA-256 verification; installers retain a direct GitHub checksum before accepting a proxied archive.
- Download JSON now reports `{items, failures}` with canonical artwork URLs, IDs, types, pages, and local paths. Partial downloads report every completed outcome and exit non-zero; no download history, cache, or cross-run de-duplication is created.
