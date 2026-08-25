# Probe Pixiv novel comments using a searched novel ID

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv comment 28853703 --type novel --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
error: pixiv:Comment: malformed_upstream_response: invalid comment time
```

Exit code: 1

Verdict: FAIL
