# Reject anonymous App-only search

## Purpose

Exercise a remaining Pixiv CLI read, download, or representative rejection path.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/anonymous-home`
- shell: `zsh 5.9`
- expected exit code: `1`
- actual exit code: `1`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv search miku --limit 1 --json --no-proxy
```

## Expected result

The CLI reports that no Pixiv account is authenticated in the anonymous HOME.

## Assertion

Exit code is 1; stderr reports unauthorized; stdout is empty.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 1
