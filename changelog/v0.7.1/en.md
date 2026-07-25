# v0.7.1 — 2026-07-25

## Changed

- Bookmark-count illustration searches now preflight the saved account's cached Pixiv Premium status (24 hours by default) and reject non-Premium accounts locally, preventing Pixiv from silently ignoring the bounds. Configure `[premium] status_cache_ttl` in `config.toml`, or run `pixiv auth refresh [UID] [--all]` to force OAuth credential and membership-cache refresh.
- Simplified text account feedback: successful login prints the compact safe account summary `✓ uid:UID username:NAME`, and `auth list` uses `*` plus `✓`/`-` local-state markers instead of `token:yes/no`.
- A missing `config.toml` is now created on the first ordinary CLI command with only common settings; advanced settings remain absent until explicitly configured, and existing files are never overwritten.
- Reformatted daily local operation logs into a compact Spring/SLF4J-style layout with timestamp, level, PID, business component, repository-relative callsite and operation; empty fields and local-only backend/status values are omitted.

## Fixed

- Restore macOS browser OAuth callbacks by making the temporary `pixiv://` helper read the same private endpoint file as the active CLI loopback listener; existing helpers rebuild automatically on the next login.
- Trigger the automatic SkillHub publication from a successful Release workflow instead of its GitHub Release event, because releases created with `github.token` do not recursively emit that event to Actions.
- Accept SkillHub's community `reviewStatus` response field when confirming an accepted submission, so a successful publish is not reported as failed.
- Skip SkillHub publication when `skills/pixiv-cli/` is unchanged from the preceding release tag, and let the Skill's own frontmatter version advance independently from the CLI release version.
