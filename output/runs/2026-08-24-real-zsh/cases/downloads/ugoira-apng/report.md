# Download discovered Ugoira as APNG

## Purpose

Exercise a remaining Pixiv CLI read, download, or representative rejection path.

## Invocation

- cwd: `/private/tmp/pixiv-cli-e2e-shell-20260821`
- HOME: `/private/tmp/pixiv-cli-e2e-shell-20260821/home`
- shell: `zsh 5.9`
- expected exit code: `0`
- actual exit code: `0`

## argv

```zsh
/private/tmp/pixiv-cli-e2e-shell-20260821/bin/pixiv download 148767248 --ugoira-mode apng --download-path /private/tmp/pixiv-cli-e2e-shell-20260821/task15-downloads/ugoira --proxy http://127.0.0.1:7890
```

## Expected result

Pixiv converts Ugoira 148767248 to APNG in the dedicated directory.

## Assertion

Exit code is 0; CLI stdout/stderr are empty; APNG extension and PNG signature are verified separately.

## Evidence

- Pixiv CLI stdout: `stdout.txt`
- Pixiv CLI stderr: `stderr.txt`

## Result

- verdict: `PASS`
- basis: actual exit code matched expected exit code 0

## Post-run evidence

- Produced files: 1
- Extension: `.apng`
- Detected MIME: `image/png`
- Pixiv CLI stdout/stderr: both empty
