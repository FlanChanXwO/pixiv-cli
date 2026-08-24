# Validate bounded Pixiv search JSON shape

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv search miku --type artwork --content-type illust --limit 3 --json --proxy http://127.0.0.1:7890 | /usr/bin/jq -e .illusts | length == 3 and all(.[]; (.id | type) == "number" and .kind == "illustration")
```

## Output
```text
zsh:1: parse error near `=='
```

Exit code: 1

Verdict: FAIL
