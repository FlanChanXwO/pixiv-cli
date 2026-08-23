# Final Pixiv CLI E2E summary

## Delivery scope

- Branch: `codex/pixiv-e2e-fixes`.
- Latest fetched `origin/main`: `e563cb64401f7c7d63d3b5dc56e23758f4bf226f`.
- Branch merge-base with `origin/main`: the same commit.
- Product repair commit: `2feb588bfb258e2938eae83d778463d493288820`.
- Real shell: zsh 5.9.
- E2E binary SHA-256: `9e8fe0f51e2f0564ca0b55ff654a8dbb67bbefcaba6e0a16551c20113aae3263`.
- Shell workspace and all case cwd values: `/private/tmp/pixiv-cli-e2e-shell-20260821`.
- Scope: Pixiv only. FANBOX, account mutations, credential rotation, interactive login, update installation, configuration writes, and the MCP stdio runtime are excluded.

## Final classification

| Classification | Count | Meaning |
| --- | ---: | --- |
| PASS | 78 | Functional/help/auth/download commands exited 0 and satisfied the audited output contract. |
| EXPECTED_FAIL | 4 | Representative invalid-ID, invalid-page, invalid-proxy, and anonymous-auth commands exited 1 as required; their case Verdict is PASS. |
| FAIL_CLI_CONTRACT | 3 | Two latest-artwork default-content cases and one MyPixiv documented-type case expose help/runtime mismatches. |
| FAIL_UPSTREAM | 10 | Nine `not_found` responses and one malformed latest-novel response were retained exactly. |
| BLOCKED | 0 | No case was prevented from running by missing credentials, tools, or environment. |
| SKIP | 0 | No recorded case was labelled SKIP; deliberate out-of-scope operations are listed separately below. |
| **Total** | **95** | Report-level totals are 82 PASS and 13 FAIL because EXPECTED_FAIL cases are successful negative tests. |

The 13 retained FAIL cases were not converted to expected-error passes and were not hidden by retries or fallback.

### CLI contract failures

- `timeline latest --type artwork` with documented default `--content-type all` returns `LatestArtworks: invalid_argument` on both recorded pages; explicit `--content-type illust` succeeds and continues across pages.
- `mypixiv works --type artwork` is documented but rejected when USER_ID is omitted; runtime `--type illust` succeeds.

### Upstream/runtime failures

- `not_found` (9): bookmark detail; two artwork-comment samples; current/discovered blocked users; two novel-detail samples; artwork-series probe; novel-series probe.
- `malformed_upstream_response` (1): latest novel timeline.

These failures remain actionable observations. They do not invalidate the three repairs explicitly requested for this branch.

## Targeted repairs

| Repair | Evidence | Result |
| --- | --- | --- |
| Current user endpoint | `cases/current-user/detail/` returned account ID `25649510` through the repaired current-user path. | PASS |
| Latest artwork continuation | Page IDs `[148810681,148810679,148810678]` and `[148810678,148810675,148810673]` are distinct; the observed boundary overlap is preserved. | PASS |
| Thumbnail MIME/extension | Thumbnail output is `.jpg` and `image/jpeg`; regular artwork is also JPEG and Ugoira APNG is `.apng` / `image/png`. | PASS |

## Evidence contract

- 95 case directories each contain exactly `report.md`, raw `stdout.txt`, and raw `stderr.txt`.
- Every case invokes the fixed Pixiv binary directly; no shell command string is evaluated.
- All 95 commands use the independent `/private/tmp` cwd; commands using `output/` as cwd: 0.
- 93 cases record an isolated HOME (92 standard cases plus the anonymous rejection). The Task 14 non-network `--version` baseline predates explicit HOME recording.
- Successful JSON documents: 38, all valid. NDJSON records: 3, all valid.
- Successful exit-0 cases with unexpected stderr: 0.
- Exit/Verdict mismatches, stream-separation errors, missing contract files, and unexpected case files: 0.
- Explicit valid-proxy cases: 56; proxy-scope errors: 0.
- Credential/token/session-cookie value hits: 0.

Historical combined-stream reports are preserved without pretending they satisfy the new case contract. All 180 are under `output/legacy/2026-08-21/`; the `output/` root contains no files. Only `output/legacy/` and `output/runs/` remain as top-level evidence directories.

## Final gates

- Targeted repair-package tests: PASS.
- `go test ./...`: PASS.
- `go vet ./...`: PASS.
- `sh scripts/build.sh`: PASS with Go 1.26.3, CGO, and darwin/arm64.
- Runner behavior test: PASS.
- Structured case, JSON/NDJSON, download-signature, proxy, secret, cwd, legacy-layout, and temporary-artifact audits: PASS.
- Code review: APPROVE; P0: 0, P1: 0, P2: 0, P3: 0.

## Explicit exclusions and residual risk

The run intentionally did not execute FANBOX, MCP stdio, bookmark/follow mutations, auth login/refresh/use/remove, config writes, or update installation. Those are not SKIP cases inside the requested read-only Pixiv matrix; they are outside scope or unsafe without separate authorization.

No stable live artwork/novel series sample was available, so both series branches are represented by transparent 404 probes rather than a fabricated success. Pixiv content and availability are time-dependent; the retained upstream failures may change on a later run.

Temporary credentials, isolated account state, downloads, and audit logs remain outside Git under `/private/tmp`. They can be removed independently after delivery; the committed reports do not depend on secret material.

## Recovery

- The legacy cleanup is a Git rename, so reverting the final delivery commit restores the previous flat paths without data loss.
- The product repair can be reverted independently from the evidence-only commits.
- No remote account state, persistent proxy setting, credential, or production configuration was changed by the final review.
