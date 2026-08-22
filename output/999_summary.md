# Pixiv CLI E2E Summary

## Scope

- Branch: `codex/pixiv-e2e-fixes`
- Repair commit: `2feb588bfb258e2938eae83d778463d493288820`
- Base: `origin/main` at `e563cb64401f7c7d63d3b5dc56e23758f4bf226f`
- Shell workspace: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- Source worktree: `/private/tmp/pixiv-cli-pixiv-e2e-fixes`
- Report directory: project `output/`; it was never used as the test, build, credential, HOME, or download directory.

## Result

The repaired branch passed the full Go test suite, `go vet ./...`, the repository build script, `git diff --check`, and a review of 23 changed files (215 additions, 41 deletions). Code review result: APPROVE, with no P0, P1, P2, or P3 findings.

Real Pixiv CLI E2E used an account exported from the locally installed `pixiv` CLI, imported through stdin into an isolated HOME. The bundle and SQLite state stayed under `/private/tmp`, permissions were checked, and no token value was written to reports.

## Repair Evidence

- Current user: user detail and current-account list paths returned authenticated profile/data through the repaired route. See `115`-`125`.
- Latest timeline: page 1/page 2 commands succeeded and produced distinct arrays using the repaired continuation path. See `106`, `107`, `114`.
- Thumbnail MIME/extension: regular and thumb downloads succeeded; both saved `.jpg` files were identified as `image/jpeg`. Ugoira APNG also succeeded and was identified as `image/png`. See `150`-`156`.

## Coverage

Covered build/help, authentication export/import/check, artwork and novel search, detail, trending tags, ranking, recommendations, following/latest timelines, MyPixiv, user search/detail/artworks/bookmarks/following/novels/related/followers, bookmark list/tags, NDJSON shell composition, regular/thumb/ugoira downloads, file signatures, invalid arguments, invalid proxy settings, missing authentication, and removed anonymous Web fallback configuration.

No bookmark/follow mutations were executed. FANBOX was excluded.

## Observed Failures

The reports retain real expected or upstream failures instead of hiding them: stale unsupported command syntax, direct-network unavailability before explicit proxy use, unavailable/invalid samples for series/comments/bookmark detail/blocked users, and audit-script quoting/regex false positives that were corrected by subsequent PASS reports. These do not invalidate the three repaired behaviors.

## Report Audit

Every report records its command, combined stdout/stderr, exit code, and verdict. Final audits found only Markdown files, no secret values, and no command using `output/` as a test workdir. Download bodies and credential material remain outside the repository.

Exit code: 0

Verdict: PASS
