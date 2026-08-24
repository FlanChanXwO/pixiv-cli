# Built Pixiv CLI version

## Purpose

Verify the real built Pixiv CLI runs from the independent zsh workspace while stdout and stderr remain separate.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `0`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv --version
```

## Expected result

The Pixiv CLI prints its development version and exits successfully.

## Assertion

Exit code is 0, stdout identifies pixiv, and stderr is empty.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0
