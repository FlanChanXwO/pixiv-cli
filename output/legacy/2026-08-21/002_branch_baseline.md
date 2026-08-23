# Verify repair branch is based on latest origin/main

## Command
```shell
cd /private/tmp/pixiv-cli-pixiv-e2e-fixes && git fetch origin main && printf "branch=%s\n" "$(git branch --show-current)" && printf "head=%s\n" "$(git rev-parse HEAD)" && printf "origin_main=%s\n" "$(git rev-parse origin/main)" && printf "merge_base=%s\n" "$(git merge-base HEAD origin/main)" && git status --short --branch
```

## Output
```text
From https://github.com/FlanChanXwO/pixiv-cli
 * branch            main       -> FETCH_HEAD
branch=codex/pixiv-e2e-fixes
head=2feb588bfb258e2938eae83d778463d493288820
origin_main=e563cb64401f7c7d63d3b5dc56e23758f4bf226f
merge_base=e563cb64401f7c7d63d3b5dc56e23758f4bf226f
## codex/pixiv-e2e-fixes...origin/codex/pixiv-e2e-fixes
?? output/
```

Exit code: 0

Verdict: PASS
