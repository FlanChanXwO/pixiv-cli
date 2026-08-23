# List discovered user novels

## Purpose

Exercise a current entity discovered earlier in this same real-zsh E2E run.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `0`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv user novels 7621567 --limit 3 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns up to three novels for user 7621567.

## Assertion

Exit code is 0; stdout is valid JSON with a novels collection.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
