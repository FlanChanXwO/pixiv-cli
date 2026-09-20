# v1.1.0 — 2026-09-20

## Added

- Expand the stable Pixiv public surface across the SDK, CLI, and MCP with complete artwork and novel bookmark operations, artwork and novel comment create/reply/stamp/delete flows, stamp discovery, novel ranking and bookmark reads, user/follow/mypixiv reads, and feed/recommendation routes backed by the App API. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- Add canonical command-scoped Pixiv target resolution and search filters so IDs, supported Pixiv URLs, entity types, artwork subtypes, bookmark bounds, dates, aspect ratio, resolution, and drawing-tool filters are validated consistently before execution. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Changed

- Stabilize the Pixiv App API, public SDK, CLI owners, and MCP owners around explicit typed contracts. Pagination now uses opaque cursors and shared continuation handling, aggregate reads preserve failure-atomic behavior, and unsupported or invalid inputs fail closed instead of silently falling back or replaying requests through another path. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- Align search, detail, series, ranking, recommendation, timeline, bookmark, comment, user, follow, and mypixiv command behavior with the verified App API contracts, including logical multi-batch traversal and stable structured output across CLI and MCP. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Fixed

- Correct Pixiv wire adapters and continuation handling for artwork/novel reads and mutations, including multi-page artwork indexes, series cursors, recommendation continuation parameters, bookmark detail normalization, and positive cursor validation. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))
- Preserve comment metadata without inventing permission semantics: current comment dates map from the verified wire field, numeric `comment_access_control` is retained under `access_control.comment_access_control`, and legacy access metadata remains compatible. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Security

- Keep Pixiv failures, cursor state, authentication decisions, and mutation boundaries fail-closed: no anonymous Web fallback is reintroduced, sensitive values remain outside public output, and replay/continuation logic validates the bound operation and inputs before issuing requests. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Documentation

- Update the English and Simplified Chinese CLI, MCP, SDK, architecture, development, README, and product-skill documentation to match the stabilized Pixiv contracts and newly exposed operations. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

## Maintenance

- Add broad offline and live-manifest regression coverage for Pixiv endpoint ownership, SDK compatibility, cursor bindings, pagination, CLI/MCP projections, comments, bookmarks, users, feeds, and mutation evidence while keeping credentials and private responses out of the repository. ([#82](https://github.com/FlanChanXwO/pixiv-cli/pull/82))

**Full Changelog**: [v1.0.2...v1.1.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.2...v1.1.0)
