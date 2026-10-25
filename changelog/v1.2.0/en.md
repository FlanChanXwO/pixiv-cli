# v1.2.0 — 2026-10-21

## Breaking changes

- Pixiv and FANBOX now share one MCP connector: `pixiv fanbox mcp` is replaced by `pixiv mcp`, and similar tools are merged behind explicit selectors such as `kind`, `relation`, and `source` (65 → 49 tools over stdio). Old tool names are removed without aliases; use the [tool migration table](https://github.com/FlanChanXwO/pixiv-cli/blob/v1.2.0/docs/en/mcp-tools.md#tool-migration). ([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146))

## Added

- `pixiv mcp --port` serves the same tools over OAuth-protected Streamable HTTP with consent/PKCE, `--host`/`--base-url`, and image/animation delivery. stdio remains the default and keeps local file downloads. ([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146))
- The HTTP mode adds the interactive `pixiv_show_gallery` MCP App: a thumbnail grid, an artwork detail view with author avatars, a zoomable full-screen original viewer, bookmark actions, exact-tag search from tags, and click-to-copy artwork IDs. ([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146), [#151](https://github.com/FlanChanXwO/pixiv-cli/pull/151))

## Changed

- Ugoira GIF/APNG encoding is now pure Go and streaming. Release binaries build with `CGO_ENABLED=0`, and source builds no longer need Rust or a C linker. Frame order, timing, transparency, resizing, and atomic output are unchanged. ([#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147))
- Configuration paths, aliases, environment overrides, and defaults are generated from struct field tags; existing keys, environment names, defaults, and generated `config.toml` content remain compatible. ([#133](https://github.com/FlanChanXwO/pixiv-cli/pull/133))

## Fixed

- Artwork search and detail output keep upstream `is_bookmarked`, `is_muted`, `visible`, `sanity_level`, and `restriction_attributes` values when known, and deep search pages start at the requested upstream offset instead of refetching earlier batches. ([#131](https://github.com/FlanChanXwO/pixiv-cli/pull/131))
- MCP `tools/list` always encodes a tool's `required` list as an array, so strict clients no longer reject the server when a tool takes no required arguments. ([#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135))

## New Contributors

- @amalimm made their first contribution. ([#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135))

**Full Changelog**: [v1.1.1...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.1...v1.2.0)

<!-- internal:
  [#134](https://github.com/FlanChanXwO/pixiv-cli/pull/134)
  [#150](https://github.com/FlanChanXwO/pixiv-cli/pull/150)
  [#122](https://github.com/FlanChanXwO/pixiv-cli/pull/122)
  [#123](https://github.com/FlanChanXwO/pixiv-cli/pull/123)
  [#124](https://github.com/FlanChanXwO/pixiv-cli/pull/124)
  [#125](https://github.com/FlanChanXwO/pixiv-cli/pull/125)
  [#137](https://github.com/FlanChanXwO/pixiv-cli/pull/137)
  [#138](https://github.com/FlanChanXwO/pixiv-cli/pull/138)
  [#144](https://github.com/FlanChanXwO/pixiv-cli/pull/144)
  [#148](https://github.com/FlanChanXwO/pixiv-cli/pull/148)
  [#149](https://github.com/FlanChanXwO/pixiv-cli/pull/149)
  [#152](https://github.com/FlanChanXwO/pixiv-cli/pull/152)
-->
