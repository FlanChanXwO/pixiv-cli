# Invalid search pagination error

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv search miku --limit 0 --page 2 --json --proxy http://127.0.0.1:7890
```

## Output
```text
error: --page requires --limit to be a positive integer
```

Exit code: 1

Verdict: FAIL
