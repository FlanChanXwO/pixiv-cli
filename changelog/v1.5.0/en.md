# v1.5.0 — 2026-11-21

## Breaking changes

- Vector indexes upgrade automatically from schema v1/v2 to v3, preserving existing records, vectors and scan checkpoints. Older binaries reject the upgraded index; back up the vector index before upgrading if you need to return to an older version. ([#305](https://github.com/FlanChanXwO/pixiv-cli/pull/305))

## Added

- Select bookmark positions with `pixiv vector sync --limit 100` for the first 100 works, `--offset 500 --limit 10` for positions 501–510, or `--tail 10` for the last 10. Public and private lists count independently, before type/rating filters. Exact tail selection traverses each selected list completely. ([#305](https://github.com/FlanChanXwO/pixiv-cli/pull/305))

- Resume a range sync by repeating the same command after interruption. Completed visibility scopes and valid vectors are reused; ordinary sync checkpoints and work outside the selection remain available. Terminal output shows range progress, sync JSON adds `range`, and `vector status --json` exposes unfinished tasks under `ranges`. ([#305](https://github.com/FlanChanXwO/pixiv-cli/pull/305))

**Full Changelog:** [v1.4.0...v1.5.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.4.0...v1.5.0)

<!-- internal: [#306](https://github.com/FlanChanXwO/pixiv-cli/pull/306) -->
