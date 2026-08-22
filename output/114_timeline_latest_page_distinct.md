# Verify Pixiv latest timeline page 2 contains a distinct result set

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && zsh ./validate-latest-pages.zsh
```

## Output
```text
page1_ids=[148767777,148767776,148767773]
page2_ids=[148767773,148767765,148767762]
page2_distinct_from_page1=PASS
```

Exit code: 0

Verdict: PASS
