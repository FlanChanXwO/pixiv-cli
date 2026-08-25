# Invalid artwork ID error

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv detail not-a-number --type artwork --json --proxy http://127.0.0.1:7890
```

## Output
```text
error: argument must be an entity ID or a supported Pixiv URL
```

Exit code: 1

Verdict: FAIL
