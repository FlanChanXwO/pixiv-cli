# v1.1.0 — 2026-09-20

## Added

- Pixiv artwork and novel bookmarks, comments (create, reply, stamp, delete, list stamps), novel ranking and bookmark reads, user/follow/MyPixiv reads, and feed/recommendation routes are available across the SDK, CLI, and MCP. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- Command inputs accept IDs, supported Pixiv URLs, entity types, artwork subtypes, bookmark ranges, dates, aspect ratio, resolution, and drawing-tool filters, and validate them before execution. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Changed

- Search, detail, series, ranking, recommendation, timeline, bookmark, comment, user, follow, and MyPixiv commands follow the verified App API contracts, including multi-batch pagination with opaque cursors. Invalid or unsupported input now fails instead of silently falling back to another request path. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Fixed

- Artwork and novel reads and mutations use corrected wire mappings, including multi-page indexes, series cursors, recommendation continuation, and bookmark detail normalization. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- Comment dates come from the verified field, `comment_access_control` is preserved, and existing access metadata stays readable. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- `pixiv auth` remote login stops its relay immediately when cancelled instead of hanging until the HTTP server shuts down. ([#86](https://github.com/FlanChanXwO/pixiv-cli/pull/86))

## Security

- Pixiv requests that fail keep failing: there is no anonymous Web fallback, sensitive values stay out of public output, and a cursor cannot be replayed against a different account or operation. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

**Full Changelog**: [v1.0.2...v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.2...v1.1.0)

<!-- internal:
  [#83](https://github.com/FlanChanXwO/pixiv-cli/pull/83)
  [#84](https://github.com/FlanChanXwO/pixiv-cli/pull/84)
  [#85](https://github.com/FlanChanXwO/pixiv-cli/pull/85)
-->
