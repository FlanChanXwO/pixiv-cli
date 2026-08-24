# List users blocked by discovered user

## Purpose

Exercise a current entity discovered earlier in this same real-zsh E2E run.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv user blocked 7621567 --limit 3 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns the blocked-user list for user 7621567 or exposes the real upstream restriction.

## Assertion

Exit code is 0 only when the upstream permits this listing; otherwise the FAIL case preserves the error.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `FAIL`
- basis: expected exit code 0, received 1
