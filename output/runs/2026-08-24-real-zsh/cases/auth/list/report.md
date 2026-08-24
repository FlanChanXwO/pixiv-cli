# List isolated Pixiv accounts

## Purpose

Prove the Task 15 cases use the isolated HOME populated from the local pixiv-cli account export.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `0`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv auth list --json
```

## Expected result

The isolated account list contains at least one account with a stored credential without exposing it.

## Assertion

Exit code is 0; JSON has a non-empty accounts array and has_token=true; no secret value is present.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
