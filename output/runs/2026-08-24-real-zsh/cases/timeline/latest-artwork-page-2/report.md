# Fetch latest artwork timeline page 2

## Purpose

Exercise the built Pixiv CLI against the real Pixiv service with the isolated account.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv timeline latest --type artwork --limit 3 --page 2 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv follows the continuation and returns the second logical page.

## Assertion

Exit code is 0; JSON contains a page distinguishable from page 1.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `FAIL`
- basis: expected exit code 0, received 1
