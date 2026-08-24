# Fetch recommended novel detail

## Purpose

Retry novel detail with a current authenticated recommendation sample after the search sample returned not_found.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv detail 28305078 --type novel --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns complete detail for recommended novel 28305078.

## Assertion

Exit code is 0; JSON novel ID is 28305078 and contains novel metadata.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `FAIL`
- basis: expected exit code 0, received 1
