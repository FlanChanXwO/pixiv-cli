# Fetch current account profile by ID

## Purpose

Exercise the built Pixiv CLI against the real Pixiv service with the isolated account.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `0`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv user detail 25649510 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns the complete profile for the isolated account ID.

## Assertion

Exit code is 0; JSON user.id equals 25649510 and contains profile fields.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
