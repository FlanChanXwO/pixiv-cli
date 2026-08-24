# Fetch latest artwork timeline page 1

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
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv timeline latest --type artwork --limit 3 --page 1 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns the first logical page of latest artwork.

## Assertion

Exit code is 0; JSON contains up to three latest illustrations.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `FAIL`
- basis: expected exit code 0, received 1
