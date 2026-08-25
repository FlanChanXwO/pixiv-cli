# Fetch latest illustration timeline page 2

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
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv timeline latest --type artwork --content-type illust --limit 3 --page 2 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv follows the repaired continuation to the second logical page.

## Assertion

Exit code is 0; JSON contains a page whose ordered IDs differ from page 1.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
