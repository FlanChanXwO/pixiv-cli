# Run a real Pixiv NDJSON search pipeline without the absent filter command

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv search miku --type artwork --limit 3 --ndjson --proxy http://127.0.0.1:7890 | /usr/bin/jq -c select(.kind == "illustration") | {id,kind}
```

## Output
```text
zsh:1: unknown file attribute: k
zsh:1: command not found: id,kind
```

Exit code: 127

Verdict: FAIL
