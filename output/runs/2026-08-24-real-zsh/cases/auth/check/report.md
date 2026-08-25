# Validate isolated Pixiv account

## Purpose

Validate the isolated saved credential against Pixiv through the explicit per-command proxy.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `0`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv auth check --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv validates the selected account and returns safe profile metadata.

## Assertion

Exit code is 0; JSON reports a positive user ID, username, and has_token=true; no credential is emitted.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
