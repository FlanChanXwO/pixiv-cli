# Task 16A novel-series validation

## Sample selection

The successful current novel search and recommendation payloads contain novel IDs but expose no series identifier. The legacy attempt to fetch one current novel detail also returns `Novel: not_found`. No valid series ID can therefore be traced from the available successful CLI evidence.

Following the Task 16A fallback, the run uses explicit probe ID `1`. It is a bounded read-only request, not a claim that ID `1` is a valid or representative series.

## Case result

- Case: `cases/series/novel-probe/`
- argv: `/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv series 1 --type novel --limit 3 --json --proxy http://127.0.0.1:7890`
- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- Expected exit: 0; actual exit: 1; verdict: FAIL.
- stdout bytes: 0.
- Raw stderr: `error: pixiv:NovelSeries: not_found`.

The result is not converted into an expected-error PASS. It proves that the real shell reached the documented novel branch and preserves the current upstream failure.

## Updated run audit

- Cases: 95; PASS: 82; FAIL: 13.
- Exactly three files per case; missing contract files: 0.
- Exit/Verdict mismatches: 0; stream-separation violations: 0.
- Successful JSON documents: 38; invalid: 0.
- NDJSON records: 3; invalid: 0.
- Explicit valid-proxy cases: 56; proxy-scope errors: 0.
- `not_found` failures: 9; `invalid_argument`: 2; malformed upstream response: 1; local help/runtime mismatch: 1.
- Secret-value hits: 0; commands using `output/` as cwd: 0.

Both `series --type artwork` and `series --type novel` are now represented. Series remains FAIL because neither probe has a valid live sample, not INCOMPLETE because a documented branch was skipped.
