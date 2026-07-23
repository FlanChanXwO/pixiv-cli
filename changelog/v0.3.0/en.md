# v0.3.0 — 2026-07-15

## Changed

- **Breaking:** the public Go SDK moved to `github.com/FlanChanXwO/pixiv-cli/pixiv`; no compatibility package remains at the former import path.
- **Breaking:** authentication accepts only raw Pixiv App API refresh tokens. Web cookies, including `refresh_token=...`, are not parsed, extracted, or converted.
- **Breaking:** `pixiv recommended` requires one of `all|illust|manga|novel|user`; `all` returns the four personalized streams atomically.

## Added

- Added MCP `recommended` with a required kind and independent pagination per stream for `all`, plus MCP `user_detail` and `pixiv user detail USER_ID` with stable structured/JSON detail output.
- Added local `--rating`, `--type`, and `--ai-type` search filters. When a page or limit is requested, matching results continue through the opaque cursor.

## Fixed

- Accepted `offset=0` in real App API recommendation cursors, correctly decoded user-detail publicity, clarified the browser login callback page, reduced non-JSON login success to one line, defaulted diagnostics to `warn`, and added an identifying User-Agent for GitHub Release update checks.

## Security

- `auth login` no longer launches managed Chromium, connects to CDP, reads browser history/session/storage, or scans existing tabs. It uses only the current loopback, controlled `pixiv://` helper, and an explicitly manual paste path.
- SDK, CLI, MCP, environment variables, and stored accounts reject cookie-shaped credentials before OAuth and never echo input content.
