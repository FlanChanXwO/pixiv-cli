# Probe Pixiv series listing with a bounded request

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv series 1 --type artwork --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
error: pixiv:ArtworkSeries: not_found
```

Exit code: 1

Verdict: FAIL
