# Create isolated shell e2e workspace

## Command
```shell
mkdir -p /private/tmp/pixiv-cli-e2e-shell-20260821/home /private/tmp/pixiv-cli-e2e-shell-20260821/credentials /private/tmp/pixiv-cli-e2e-shell-20260821/downloads /private/tmp/pixiv-cli-e2e-shell-20260821/tmp /private/tmp/pixiv-cli-e2e-shell-20260821/legacy-output && chmod 700 /private/tmp/pixiv-cli-e2e-shell-20260821 /private/tmp/pixiv-cli-e2e-shell-20260821/home /private/tmp/pixiv-cli-e2e-shell-20260821/credentials /private/tmp/pixiv-cli-e2e-shell-20260821/downloads /private/tmp/pixiv-cli-e2e-shell-20260821/tmp /private/tmp/pixiv-cli-e2e-shell-20260821/legacy-output && if [[ -f /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output/900_goal_recovery_inventory.md ]]; then mv /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output/900_goal_recovery_inventory.md /private/tmp/pixiv-cli-e2e-shell-20260821/legacy-output/; fi && ls -ld /private/tmp/pixiv-cli-e2e-shell-20260821 /private/tmp/pixiv-cli-e2e-shell-20260821/*
```

## Output
```text
drwx------@ 7 flanchan  wheel  224 Aug 21 19:14 /private/tmp/pixiv-cli-e2e-shell-20260821
drwx------@ 2 flanchan  wheel   64 Aug 21 19:14 /private/tmp/pixiv-cli-e2e-shell-20260821/credentials
drwx------@ 2 flanchan  wheel   64 Aug 21 19:14 /private/tmp/pixiv-cli-e2e-shell-20260821/downloads
drwx------@ 2 flanchan  wheel   64 Aug 21 19:14 /private/tmp/pixiv-cli-e2e-shell-20260821/home
drwx------@ 3 flanchan  wheel   96 Aug 21 19:14 /private/tmp/pixiv-cli-e2e-shell-20260821/legacy-output
drwx------@ 2 flanchan  wheel   64 Aug 21 19:14 /private/tmp/pixiv-cli-e2e-shell-20260821/tmp
```

Exit code: 0

Verdict: PASS
