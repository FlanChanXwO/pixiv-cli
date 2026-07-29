# v0.8.0 Review

## Scope and method

Three fresh, read-only reviews covered runtime correctness, release/document
contracts, and the final corrected diff. Findings were verified against the
working tree before changes.

## Resolved findings

- **P1 ClawHub publication:** use the supported `clawhub --cli-version` flag;
  publish the exact tag version with repository/ref/commit/path provenance; use
  `skill verify` rather than unsupported `inspect` provenance assumptions.
- **P1 authenticated CLI E2E:** the protected temporary token is now supplied
  only to `pixiv auth import` on stdin in an isolated HOME. Data commands never
  receive it through `PIXIV_REFRESH_TOKEN`.
- **P1/P2 account-pool download safety:** typed `Retry-After` survives resource
  mapping and download reporting. Partial static downloads explicitly mark
  `Committed`, so a later 429 cannot replay already published files with a
  second account.
- **P2 real-test scope:** SDK network canaries require the separate
  `PIXIV_E2E_SDK=1` opt-in; release real E2E is CLI-only. The CLI R18 canary
  verifies both GIF and APNG extensions and magic bytes.
- **P2 encoder convergence:** the legacy embedded download-manager interface
  is now an adapter over `internal/ugoira`, so public SDK, CLI and MCP random
  downloads use the same runtime encoder.
- **P2 release/docs drift:** three CLI references, MCP record descriptions,
  repository guidance and README skill-discovery text were updated for local
  account selection, managed config aliases, feeds/filter, APNG and ClawHub.
- **Major text writer error propagation:** every text list printer now returns
  `io.Writer` failures. Search/detail command tests prove a failing stdout
  produces a non-zero exit; user, recommendation and spool output paths use
  the same rule.
- **Major false-429 replay boundary:** an output attempt is marked committed
  *before* encoding, temporary-document copy, heading, or text printing. A
  downstream writer that returns a typed Pixiv `Retry-After` error therefore
  cannot be mistaken for an upstream rate limit and cause account-pool replay.
  Regressions cover both normal search and `recommended all --ndjson`.
- **Major stale end-to-end configuration contract:** the offline binary E2E
  now exercises an allowed `filename_template` alias and explicitly verifies
  that removed `output_json` is rejected.

## Final review result

The final independent review found no remaining Critical, Major, P1, or Minor
issues in scope. Download's successful stdout is intentionally empty: it is a
v0.8 action contract, and the Web fallback E2E verifies exit status plus files
instead of the obsolete text summary.

## Evidence run after fixes

- `go test ./e2e ./internal/cli ./internal/application ./pixiv -count=1`
- `go test ./internal/download ./pixiv ./e2e ./internal/cli -count=1`
- `go test ./scripts/clawhubworkflow ./scripts/documentation -count=1`
- `go test ./scripts/clawhubworkflow ./scripts/releaseworkflow ./scripts/documentation -count=1`
- `go test ./e2e -count=1`
- `go vet ./...`
- `go test -race ./...`
- `sh scripts/build.sh`
- `sh scripts/test-release-workflow.sh`
- `sh scripts/test-homebrew-prepublish-workflow.sh`
- `pre-commit run --all-files`
- YAML parsing for `.github/workflows/publish-clawhub.yml`
- `git diff --check`

The real CLI Web fallback E2E was also attempted both with the configured
proxy and directly. Pixiv returned a connection reset through the proxy and a
timeout directly; neither route exercised login, writes, or credentials.
