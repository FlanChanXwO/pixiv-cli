# Fetch discovered artwork detail

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
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv detail 148810634 --type artwork --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns the complete detail for artwork 148810634.

## Assertion

Exit code is 0; JSON artwork ID is 148810634 and contains pages/resource metadata.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
