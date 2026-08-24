# Invalid proxy URL error

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv search miku --limit 1 --json --proxy http://[invalid
```

## Output
```text
zsh:1: bad pattern: http://[invalid
```

Exit code: 1

Verdict: FAIL
