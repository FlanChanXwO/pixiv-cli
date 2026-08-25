# Final pre-delivery validation

## Remote baseline

`git fetch origin` completed successfully before this validation. `origin/main` is `e563cb64401f7c7d63d3b5dc56e23758f4bf226f`, equals the branch merge-base, and is an ancestor of the delivery branch. `origin/codex/pixiv-e2e-fixes` is also an ancestor, so the final push can be fast-forward and requires no history rewrite.

## Regression gates

| Check | Result |
| --- | --- |
| `zsh scripts/e2evidence/run-case_test.zsh` | PASS |
| Targeted user-detail, timeline, public Pixiv SDK, downloader, and MCP-download tests | PASS |
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| `sh scripts/build.sh` | PASS |

The standard build used Go 1.26.3 with CGO on darwin/arm64. The E2E binary differs from the standard-build hash only because the standard script adds `-trimpath`; module and dependency metadata match, and no Go/product source changed after the repair commit.

Product/non-output `git diff --check`, the evidence runner, and `output/runs/` pass without findings. A whole-branch check that includes archived raw transcripts reports exactly one pre-existing trailing-whitespace line in `output/legacy/2026-08-21/174_review_scope_check.md`; that file is the retained output of a failed historical awk audit. The line is preserved for traceability and excluded from formatting gates for current code and structured evidence rather than silently rewritten.

## Evidence and security gates

| Metric | Result |
| --- | ---: |
| Case directories/reports/stdout/stderr | 95 / 95 / 95 / 95 |
| PASS / retained FAIL | 82 / 13 |
| Contract, unexpected-file, cwd, argv, Verdict, stream errors | 0 |
| Successful JSON documents / invalid | 38 / 0 |
| NDJSON records / invalid | 3 / 0 |
| Successful commands with unexpected stderr | 0 |
| Explicit valid-proxy cases / proxy-scope errors | 56 / 0 |
| Strict credential/header/session-cookie value hits | 0 |
| Commands using `output/` as cwd | 0 |
| Tracked temporary credential/download/audit artifacts | 0 |
| Root-level output files | 0 |
| Legacy Markdown reports | 180 |

Retained failures classify as nine `not_found`, two `invalid_argument`, one malformed upstream response, and one local help/runtime mismatch. The four negative tests are expected failures at the command level and PASS at the case-contract level.

## Pre-push conclusion

The branch is ready for a normal fast-forward push. Remote HEAD, ancestry, final report paths, and secret/layout invariants must be queried again after the final commit is pushed; those post-push observations cannot truthfully be embedded in the commit they verify.
