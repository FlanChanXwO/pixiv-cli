# Pixiv CLI real-zsh E2E run

- Run ID: `2026-08-24-real-zsh`
- Source worktree: `/private/tmp/pixiv-cli-pixiv-e2e-fixes`
- Shell workspace: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- Shell: zsh 5.9
- Binary: `/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv`
- Binary SHA-256: `9e8fe0f51e2f0564ca0b55ff654a8dbb67bbefcaba6e0a16551c20113aae3263`
- Authentication: account exported from the local installed pixiv-cli, restored into an isolated HOME; no secret was logged.
- Network: explicit per-command proxy `http://127.0.0.1:7890` after the previously observed direct-network failure.
- Scope: Pixiv CLI only; FANBOX, MCP runtime, interactive login, credential refresh, and account-mutating operations are excluded.
- Evidence contract: one logical Pixiv CLI invocation per case directory, with `report.md`, raw `stdout.txt`, and raw `stderr.txt`.

## Result

95 cases were recorded: 82 PASS and 13 retained FAIL. PASS includes four representative rejection tests with expected exit code 1. All 38 successful JSON documents and three NDJSON records are valid; no successful command wrote unexpected stderr. Task 16A invoked the previously missing novel branch of `pixiv series`; both artwork and novel probes currently return upstream `not_found`. The run awaits final review and delivery in Task 17. See `task15-validation.md`, `task16-audit.md`, and `task16a-validation.md`.

## Coverage groups

| Group | Result |
| --- | --- |
| Current binary help | 35/35 PASS |
| Isolated authentication | list/check PASS |
| Search/discovery/ranking | artwork JSON, artwork NDJSON, Ugoira, trending tags, novel, user, daily ranking PASS |
| Recommendations | artwork, novel, user, all PASS |
| Timeline | following artwork/novel PASS; corrected latest artwork pages 1/2 PASS; default-all artwork and latest novel retained FAIL |
| MyPixiv | users, works illust, works novel PASS; documented `artwork` spelling retained FAIL |
| Current/discovered users | detail and read-only lists PASS except blocked-user upstream 404 |
| Detail/comments/bookmarks/series | artwork/user detail and novel comments PASS; artwork and novel series probes plus remaining upstream 404s retained |
| Downloads | regular JPEG, thumb JPEG, Ugoira APNG PASS with file signatures |
| Error paths | invalid ID, invalid pagination, invalid proxy, anonymous auth requirement all rejected as expected |

## Targeted repairs

| Repair | Evidence | Result |
| --- | --- | --- |
| Current user endpoint | `cases/current-user/detail/` returned isolated account ID `25649510` | PASS |
| Latest timeline continuation | `cases/timeline/latest-artwork-illust-page-{1,2}/` returned distinct ordered ID arrays | PASS |
| Thumbnail MIME/extension | `cases/downloads/thumb/` produced `.jpg` detected as `image/jpeg` | PASS |
