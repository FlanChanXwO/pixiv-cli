# Removed web fallback setting rejection

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/anonymous-home ./bin/pixiv config set web_fallback_enabled true
```

## Output
```text
error: removed_setting: config key "web_fallback_enabled" was removed; clear it with `pixiv config unset web_fallback_enabled`
```

Exit code: 1

Verdict: FAIL
