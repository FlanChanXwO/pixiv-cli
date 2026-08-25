# Reject invalid artwork ID

## Purpose

Exercise a remaining Pixiv CLI read, download, or representative rejection path.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `1`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv detail not-a-number --type artwork --json --proxy http://127.0.0.1:7890
```

## Expected result

The CLI rejects a non-numeric entity reference before a network request.

## Assertion

Exit code is 1; stderr states that the argument must be an entity ID or supported Pixiv URL; stdout is empty.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 1
