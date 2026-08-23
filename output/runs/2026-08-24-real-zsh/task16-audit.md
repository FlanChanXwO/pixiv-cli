# Task 16 concentrated audit

## Conclusion

The structured evidence contract is internally consistent and the three targeted repairs remain supported. The run is not yet complete enough for final delivery: `pixiv series` advertises both `artwork` and `novel`, but the new run invokes only the artwork branch. A separate follow-up task must record a novel-series case before Task 17.

No Pixiv case was rerun or rewritten during this audit. The 12 retained FAIL cases remain raw observations rather than being relabelled as PASS.

## Evidence contract

An independent audit derived the case set from `cases/*/*/report.md` and checked every leaf directory:

- 94 case directories and 94 reports; every directory contains exactly `report.md`, `stdout.txt`, and `stderr.txt`.
- 82 PASS and 12 FAIL; 78 PASS cases exited 0 and four expected-rejection PASS cases exited 1.
- Required headings, cwd, argv, expected/actual exit codes, evidence links, verdict, and basis are present exactly once; contract errors: 0.
- All argv entries invoke `/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv` directly.
- All 94 commands use cwd `/private/tmp/pixiv-cli-e2e-shell-20260821`; commands using `output/` as cwd: 0.
- Verdict/exit mismatches: 0. Stream-separation violations: 0. Unexpected case files: 0.
- The Task 14 non-authenticated `--version` baseline predates explicit HOME recording. The other 93 reports record an isolated HOME: 92 cases use `.../home`, and the anonymous rejection case uses `.../anonymous-home`.

The runner behavior test passed again, including direct argv execution, stdout/stderr separation, explicit HOME injection, and rejection of a report-directory cwd.

## Formats, repairs, and failures

The Task 15 shape validator passed again: 38 successful JSON documents and all three NDJSON records parse; successful exit-0 cases with non-empty stderr: 0; missing case artifacts: 0.

The targeted evidence remains unchanged:

- current user: profile ID `25649510`, matching the isolated account;
- latest continuation: page ID arrays `[148810681,148810679,148810678]` and `[148810678,148810675,148810673]` are distinct while preserving the observed boundary overlap;
- media: regular and thumbnail files are `.jpg` / `image/jpeg`; Ugoira APNG is `.apng` / `image/png`.

The 12 FAIL cases classify exactly as follows:

- eight Pixiv `not_found` responses: bookmark detail, two artwork-comment samples, two blocked-user routes, two novel-detail samples, and the artwork-series probe;
- two `LatestArtworks: invalid_argument` responses when the documented default `--content-type all` is used;
- one `LatestNovels: malformed_upstream_response`;
- one local help/runtime mismatch where `mypixiv works --type artwork` is documented but the runtime requires `illust` when USER_ID is omitted.

The latter two CLI contract mismatches are product findings, but they are outside the three repairs explicitly requested in this goal. They remain visible for the final report and were not silently fixed or retried away.

## Isolation, proxy, and secrets

- 55 cases use the explicit per-command proxy `http://127.0.0.1:7890`.
- All network cases requiring a valid proxy contain that exact argv option. Local help/version/auth-list cases do not; the anonymous-auth case explicitly uses `--no-proxy`; the invalid-proxy rejection intentionally uses `ftp://127.0.0.1:7890`.
- No persistent proxy setting was changed.
- Secret-value scan hits across credential tokens, authorization headers, and session cookies: 0.
- The isolated config and database are outside the repository with mode `0600`.
- The three downloaded files are outside the repository with mode `0600`; temporary authentication/download/audit paths tracked by Git: 0.

## Branch, build, and regression gates

- `git merge-base HEAD origin/main` and `origin/main` both resolve to `e563cb64401f7c7d63d3b5dc56e23758f4bf226f`.
- No Go/product source changed after repair commit `2feb588bfb258e2938eae83d778463d493288820`; later changes are evidence and runner files only.
- `zsh scripts/e2evidence/run-case_test.zsh`: PASS.
- `go test ./...`: PASS.
- `go vet ./...`: PASS.
- `sh scripts/build.sh`: PASS (`go1.26.3`, `darwin/arm64`).

The E2E binary hash is `9e8fe0f51e2f0564ca0b55ff654a8dbb67bbefcaba6e0a16551c20113aae3263`. The standard build has a different hash because `scripts/build.sh` adds `-trimpath`; `go version -m` shows the same module/dependency metadata and records `-trimpath=true` only on the standard build. The original build command and hash remain in the legacy build evidence.

## Scope alignment

FANBOX, MCP stdio, interactive login, credential refresh/rotation, configuration writes, updates, and bookmark/follow mutations remain excluded as required. No account-mutating command was executed.

The only newly identified evidence gap is the uninvoked novel branch of `pixiv series`. This audit therefore appends a focused Task 16A rather than declaring the run complete or entering final delivery early.

## Task 16A follow-up

Task 16A subsequently invoked `pixiv series 1 --type novel --limit 3 --json` through the same runner, isolated HOME, cwd, binary, and explicit proxy. Pixiv returned `NovelSeries: not_found`; the case is retained as FAIL with empty stdout and raw stderr. The run now contains 95 cases: 82 PASS and 13 FAIL. Both documented series type branches have been invoked, so the coverage gap identified above is closed even though no successful series sample was available.
