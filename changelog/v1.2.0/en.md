# v1.2.0 — 2026-10-21

## Breaking changes

- Pixiv and FANBOX now share one MCP connector: `pixiv fanbox mcp` is replaced by `pixiv mcp`, and homogeneous MCP tools are consolidated (stdio 65 → 49 tools) behind explicit selectors such as `kind`, `relation` and `source`. Old tool names are unregistered rather than aliased; follow the [tool migration table](https://github.com/FlanChanXwO/pixiv-cli/blob/v1.2.0/docs/en/mcp-tools.md#tool-migration). ([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146))

## Added

- `pixiv mcp --port` serves the same tools over OAuth-protected Streamable HTTP with owner consent/PKCE, `--host`/`--base-url`, helper login and capability-scoped image/animation delivery; stdio remains the default and keeps local filesystem downloads. ([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146))
- Add the interactive `pixiv_show_gallery` MCP App for HTTP mode: a thumbnail grid and an artwork detail view with author avatars, a full-screen original-image viewer with zoom and progressive loading, bookmark actions, clickable tags that start an exact-tag search, and click-to-copy artwork IDs. ([#146](https://github.com/FlanChanXwO/pixiv-cli/pull/146), [#151](https://github.com/FlanChanXwO/pixiv-cli/pull/151))
- Add the offline `pixiv --licenses` option, which prints the third-party notices embedded in the executable. ([#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147))

## Changed

- Ugoira GIF/APNG encoding is now a streaming pure-Go implementation; release binaries are built with `CGO_ENABLED=0` and source builds no longer need Rust or a C linker. Frame order, timing, transparency, resizing and atomic output are preserved. ([#147](https://github.com/FlanChanXwO/pixiv-cli/pull/147))

## Fixed

- Artwork search and detail output preserve upstream `is_bookmarked`, `is_muted`, `visible`, `sanity_level` and `restriction_attributes` values, omitting them only when unknown; unfiltered deep search pages start at the raw upstream offset instead of refetching earlier batches. ([#131](https://github.com/FlanChanXwO/pixiv-cli/pull/131))
- MCP `tools/list` always encodes an input schema's `required` list as an array, so strict clients no longer reject the server when a tool has no required arguments. ([#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135))

## Documentation

- Add and later remove the Telegram group badge and link in the README. ([#134](https://github.com/FlanChanXwO/pixiv-cli/pull/134), [#150](https://github.com/FlanChanXwO/pixiv-cli/pull/150))

## Maintenance

- Derive configuration paths, aliases, environment overrides and defaults from struct field tags, keeping existing keys, environment names, defaults and initial-file bytes compatible. ([#133](https://github.com/FlanChanXwO/pixiv-cli/pull/133))
- Recover downstream release publishers, verify Homebrew on Linux, and harden trusted change classification and monotonic Homebrew recovery. ([#122](https://github.com/FlanChanXwO/pixiv-cli/pull/122), [#123](https://github.com/FlanChanXwO/pixiv-cli/pull/123), [#124](https://github.com/FlanChanXwO/pixiv-cli/pull/124))
- Restore PR triage automation, scope PR metadata permissions, find smoke workers across check suites, run Quality after base edits, and support pure-Go heads in trusted smoke workers. ([#125](https://github.com/FlanChanXwO/pixiv-cli/pull/125), [#137](https://github.com/FlanChanXwO/pixiv-cli/pull/137), [#138](https://github.com/FlanChanXwO/pixiv-cli/pull/138), [#144](https://github.com/FlanChanXwO/pixiv-cli/pull/144), [#148](https://github.com/FlanChanXwO/pixiv-cli/pull/148))
- Restrict permanent test additions, keep PR scope focused, and prepare the matching v1.2.0 release metadata. ([#149](https://github.com/FlanChanXwO/pixiv-cli/pull/149), [#152](https://github.com/FlanChanXwO/pixiv-cli/pull/152))

## New Contributors

- @amalimm made their first contribution in [#135](https://github.com/FlanChanXwO/pixiv-cli/pull/135)

**Full Changelog**: [v1.1.1...v1.2.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.1.1...v1.2.0)
