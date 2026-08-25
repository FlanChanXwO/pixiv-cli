# List comments for a ranked artwork

## Purpose

Exercise a remaining Pixiv CLI read, download, or representative rejection path.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv comment 148682428 --type artwork --limit 3 --json --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv returns up to three comments for ranked artwork 148682428, including an empty list.

## Assertion

Exit code is 0 and stdout is valid comments JSON, or the FAIL preserves the upstream error.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `FAIL`
- basis: expected exit code 0, received 1
