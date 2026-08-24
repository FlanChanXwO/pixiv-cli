# Task 15 validation, updated after Task 16A

The validation reads the structured case artifacts and the temporary download files. It does not alter Pixiv state and does not copy non-Pixiv audit output into any case `stdout.txt` or `stderr.txt`.

## Contract and format

- Case count: 95
- PASS: 82
- FAIL: 13
- Missing `report.md` / `stdout.txt` / `stderr.txt`: 0
- Help cases: 35; missing `Usage:` or non-empty stderr: 0
- Successful JSON documents checked with `jq`: 38; invalid: 0
- NDJSON records checked: 3; invalid: 0
- Successful Pixiv CLI cases with unexpected stderr: 0
- Commands whose cwd is under `output/`: 0
- Credential/token/Cookie value pattern hits: 0

`PASS` includes four representative invalid-input/auth cases whose expected result is a non-zero rejection. The 13 `FAIL` cases are retained real upstream or runtime-contract failures, not hidden retries.

## Targeted repair evidence

- Current user route: `current-user/detail` returned user ID `25649510`, matching the isolated account.
- Latest timeline continuation: page 1 IDs were `[148810681,148810679,148810678]`; page 2 IDs were `[148810678,148810675,148810673]`. The ordered pages are distinct; the observed boundary overlap is preserved.
- Media MIME/extension: regular and thumb each produced one `.jpg` detected as `image/jpeg`; Ugoira produced one `.apng` detected as `image/png`.

## Retained failures

- Help/runtime mismatch: `timeline latest --type artwork` with default content type returns `invalid_argument`; explicit `--content-type illust` succeeds for pages 1 and 2.
- Help/runtime mismatch: `mypixiv works --type artwork` is rejected; runtime-supported `--type illust` succeeds.
- Upstream/runtime failures: latest novel is `malformed_upstream_response`; novel detail samples, artwork comments, bookmark detail, blocked users, and both artwork/novel series probes return `not_found`.

These failures do not invalidate the three targeted repairs, but they remain visible for final classification.
