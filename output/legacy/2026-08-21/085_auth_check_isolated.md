# Validate imported Pixiv account through network

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv auth check 25649510 --json
```

## Output
```text
error: pixiv:Open: upstream_unavailable: pixiv upstream transport failed
```

Exit code: 1

Verdict: FAIL
