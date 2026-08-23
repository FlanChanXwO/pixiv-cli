# Final code review

## Code Review Summary

- Review range: `origin/main...f7598392a49acc76064441f12d3ecd3c743526bc` plus the pending legacy-report renames and final summaries.
- Files reviewed before final summaries: 487 total; 26 non-output product, test, documentation, and runner files plus 461 evidence files.
- Overall assessment: **APPROVE**.
- LSP note: no callable LSP tool was available in this environment. Definition/reference checks used targeted `rg`, Go compilation, unit tests, vet, and full regression instead.

## Findings

### P0 - Critical

None.

### P1 - High

None.

### P2 - Medium

None.

### P3 - Low

None.

## What was checked

- Current-user routing now uses `/v1/user/detail` with `user_id=0` and the Android filter; the removed `/v1/user/me` constant has no production reference.
- Latest-artwork continuation preserves either `max_illust_id` or legacy `offset`, rejects ambiguous/duplicate continuation values, binds cursors to operation/query/account context, and sends the decoded key on the next request.
- `SavedResource.ContentType` only exposes an allowlisted response header. Thumbnail/mini extension correction accepts a fixed image-media allowlist, falls back to file signatures, never derives a path suffix from untrusted free-form input, does not overwrite an existing target through the hard-link publish step, and rolls back the new link if removal of the old path fails.
- MCP download MIME output uses file signatures for supported image types and falls back to the established extension mapping.
- Unit tests cover current-user route/query, latest continuation propagation and ambiguity rejection, content-signature MIME, and real thumbnail extension publication. Real zsh evidence covers all three repairs.
- The evidence runner executes argv directly, constrains cwd and explicit HOME to `/private/tmp`, rejects the report tree as cwd/HOME, refuses case overwrite, and separates Pixiv stdout/stderr.
- Auth and output artifacts were scanned for secret values; temporary credentials/downloads are outside Git with private permissions.
- English and Simplified Chinese release notes, SDK behavior, CLI download semantics, and maintainer architecture documentation are synchronized. These are the only locale trees present in this repository.

## Architecture and quality

The changes preserve existing package ownership: App API adapters remain under `internal/services/pixiv`, public behavior remains in `sdk/pixiv`, disk publication remains in `internal/media/downloader`, and MCP only adapts the downloader result. No new dependency, persistent configuration, silent fallback, arbitrary timeout, retry cap, or data truncation was introduced.

Error paths remain explicit: malformed continuations fail, file-publication failures preserve or roll back the original artifact, unsupported MIME stays on the prior extension behavior, and real upstream errors remain visible in E2E reports.

## Removal/iteration plan

No production code is a safe-removal candidate in this change. The only removal action is evidence organization: 180 obsolete root-level reports were moved, without content loss, to `output/legacy/2026-08-21/`. Their combined-stream format is retained for traceability but excluded from new-run metrics.

## Areas intentionally not covered

- No account-mutating, credential-rotating, interactive-login, MCP-runtime, update-installation, or FANBOX operation was executed.
- No database migration or UI surface changed.
- Series success could not be verified because both explicit live probes returned 404; this remains visible rather than being downgraded or masked.

## Verification

Targeted tests, full `go test ./...`, `go vet ./...`, standard build, runner test, evidence-contract audits, secret scans, and download signature checks all pass.
