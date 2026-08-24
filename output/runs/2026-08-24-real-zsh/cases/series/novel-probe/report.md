# Pixiv novel series probe

## Purpose

Invoke the documented novel branch of the series command with a bounded read-only request when no valid series ID is exposed by current successful novel responses.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv series 1 --type novel --limit 3 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns a bounded JSON novel-series collection for the supplied series ID.

## Assertion

Exit code is 0 and stdout is valid JSON containing a novel-series collection; otherwise retain the exact upstream failure.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `FAIL`
- basis: expected exit code 0, received 1
