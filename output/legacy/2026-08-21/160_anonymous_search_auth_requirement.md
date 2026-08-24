# Anonymous search authentication requirement

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/anonymous-home ./bin/pixiv search miku --limit 1 --json --no-proxy
```

## Output
```text
error: pixiv:auth: unauthorized: no pixiv account is authenticated
```

Exit code: 1

Verdict: FAIL
